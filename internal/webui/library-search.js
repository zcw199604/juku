export function normalizeSearchText(value) {
  const text = String(value ?? '').normalize('NFKC').toLowerCase().trim();

  return text.replace(/[\p{P}\s]+/gu, '') || text;
}

export function searchTerms(query) {
  return [...new Set(String(query ?? '').trim().split(/\s+/u).map(normalizeSearchText).filter(Boolean))];
}

export function matchesSearch(text, query, terms = searchTerms(query)) {
  const normalized = normalizeSearchText(text), keyword = normalizeSearchText(query);
  return !keyword || normalized.includes(keyword) || terms.length > 1 && terms.every(term => normalized.includes(term));
}

export function rankSearchResults(list, query, textFor, onlineIDs = new Set()) {
  const keyword = normalizeSearchText(query), terms = searchTerms(query);
  if (!keyword) return list.slice();
  function rank(drama) {
    const title = normalizeSearchText(drama.title || drama.name);
    if (title === keyword) return 0;
    if (title.startsWith(keyword)) return 1;
    if (title.includes(keyword)) return 2;
    if (terms.length > 1 && terms.every(term => title.includes(term))) return 3;
    if (matchesSearch(textFor(drama), query, terms)) return 4;
    return onlineIDs.has(drama.id) ? 5 : 6;
  }
  return list.map((drama, index) => ({drama, index, rank: rank(drama)}))
    .sort((left, right) => left.rank - right.rank || left.index - right.index).map(item => item.drama);
}
