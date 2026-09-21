export function createSuggestionScheduler({load, changed, delay = 300, now = Date.now}) {
  const cache = new Map();
  let sequence = 0, timer, deadline, controller;
  function cancel() {
    sequence++;
    clearTimeout(timer);
    clearTimeout(deadline);
    controller?.abort();
    controller = null;
  }
  function clear() {cancel(); changed({query: '', items: [], loading: false});}
  function schedule(raw) {
    cancel();
    const query = raw.trim(), current = sequence;
    changed({query, items: [], loading: false});
    if (!query || Array.from(query).length > 80 || /[\u0000-\u001f\u007f-\u009f]/.test(query)) return;
    const cached = cache.get(query);
    if (cached && now() < cached.expires) {
      changed({query, items: cached.items, loading: false});
      return;
    }
    cache.delete(query);
    timer = setTimeout(async () => {
      const request = new AbortController();
      controller = request;
      const timeout = setTimeout(() => request.abort(), 6000);
      deadline = timeout;
      changed({query, items: [], loading: true});
      try {
        const result = await load(query, request.signal);
        if (current !== sequence || request.signal.aborted) return;
        if (!Array.isArray(result)) throw new Error('Invalid search suggestions');
        const seen = new Set(), items = [];
        for (const row of result) {
          const name = typeof row?.name === 'string' ? row.name.trim() : '';
          if (!name || Array.from(name).length > 80 || /[\u0000-\u001f\u007f-\u009f]/.test(name) || seen.has(name)) continue;
          seen.add(name);
          items.push({name});
          if (items.length === 10) break;
        }
        for (const [key, value] of cache) if (now() >= value.expires) cache.delete(key);
        if (cache.size >= 32) cache.delete(cache.keys().next().value);
        cache.set(query, {items, expires: now() + 60000});
        changed({query, items, loading: false});
      } catch (_) {
        if (current === sequence) changed({query, items: [], loading: false});
      } finally {
        clearTimeout(timeout);
        if (current === sequence) {
          controller = null;
          if (request.signal.aborted) changed({query, items: [], loading: false});
        }
      }
    }, delay);
  }
  return {schedule, clear};
}

export function suggestionParts(name, query) {
  const matches = new Set(Array.from(query).filter(char => char.trim()).map(char => char.toLowerCase()));
  const parts = [];
  for (const char of name) {
    const highlighted = matches.has(char.toLowerCase()), last = parts[parts.length - 1];
    if (last && last.highlighted === highlighted) last.text += char;
    else parts.push({text: char, highlighted});
  }
  return parts;
}

export function createSearchSuggestions({input, panel, list, enabled, load, select, submit, searchIcon}) {
  const doc = input.ownerDocument, view = doc.defaultView;
  let items = [], active = -1, composing = false;
  const scheduler = createSuggestionScheduler({load, changed: render});

  function close() {scheduler.clear();}
  function refresh() {
    if (composing || !enabled() || doc.activeElement !== input) {close(); return;}
    scheduler.schedule(input.value);
  }
  function fitPanel() {
    if (panel.hidden) return;
    const viewport = view.visualViewport;
    const bottom = viewport ? viewport.height + viewport.offsetTop : view.innerHeight;
    panel.style.maxHeight = Math.max(100, Math.min(380, bottom - panel.getBoundingClientRect().top - 12)) + 'px';
  }
  function activate(index) {
    active = index;
    Array.from(list.children).forEach((option, position) => option.setAttribute('aria-selected', String(position === active)));
    if (active < 0) {input.removeAttribute('aria-activedescendant'); return;}
    const option = list.children[active];
    input.setAttribute('aria-activedescendant', option.id);
    option.scrollIntoView({block: 'nearest'});
  }
  function choose(index) {
    const item = items[index];
    if (!item || !enabled() || composing) return;
    close();
    input.value = item.name;
    select(item.name);
  }
  function render(state) {
    const valid = !composing && enabled() && doc.activeElement === input && state.query === input.value.trim();
    items = valid ? state.items : [];
    active = -1;
    input.removeAttribute('aria-activedescendant');
    input.setAttribute('aria-busy', String(valid && state.loading));
    input.setAttribute('aria-expanded', String(items.length > 0));
    panel.hidden = !items.length;
    list.replaceChildren();
    items.forEach((item, index) => {
      const option = doc.createElement('button');
      option.type = 'button';
      option.tabIndex = -1;
      option.id = list.id + '-' + index;
      option.className = 'search-suggestion';
      option.setAttribute('role', 'option');
      option.setAttribute('aria-selected', 'false');
      const label = doc.createElement('span');
      label.className = 'search-suggestion-name';
      for (const part of suggestionParts(item.name, state.query)) {
        const text = doc.createElement(part.highlighted ? 'mark' : 'span');
        text.textContent = part.text;
        label.appendChild(text);
      }
      option.append(searchIcon(), label);

      option.addEventListener('mousedown', event => {if (event.button === 0) event.preventDefault();});
      option.addEventListener('click', () => choose(index));
      list.appendChild(option);
    });
    fitPanel();
  }

  input.addEventListener('input', refresh);
  input.addEventListener('focus', refresh);
  input.addEventListener('blur', close);
  input.addEventListener('compositionstart', () => {composing = true; close();});
  input.addEventListener('compositionend', () => {composing = false; refresh();});
  input.addEventListener('keydown', event => {
    if (composing || event.isComposing || event.keyCode === 229) return;
    if (event.key === 'Escape') {
      if (!panel.hidden) event.preventDefault();
      close();
    } else if (event.key === 'Tab') close();
    else if ((event.key === 'ArrowDown' || event.key === 'ArrowUp') && items.length) {
      event.preventDefault();
      const next = event.key === 'ArrowDown' ? (active + 1) % items.length : (active < 0 ? items.length - 1 : (active + items.length - 1) % items.length);
      activate(next);
    } else if (event.key === 'Enter') {
      event.preventDefault();
      if (active >= 0 && !panel.hidden) choose(active);
      else {close(); submit();}
    }
  });
  doc.addEventListener('pointerdown', event => {if (event.target !== input && !panel.contains(event.target)) close();});
  doc.addEventListener('visibilitychange', () => {if (doc.hidden) close();});
  view.addEventListener('resize', fitPanel);
  view.visualViewport?.addEventListener('resize', fitPanel);
  return {close, refresh};
}
