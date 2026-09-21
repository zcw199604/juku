const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const source = fs.readFileSync(path.join(__dirname, 'search-suggestions.js'), 'utf8');
const modulePromise = import('data:text/javascript;base64,' + Buffer.from(source).toString('base64'));
const tick = () => new Promise(resolve => setTimeout(resolve, 5));
const flush = () => new Promise(resolve => setImmediate(resolve));

test('typing a burst only requests the latest complete keyword', async () => {
  const {createSuggestionScheduler} = await modulePromise;
  const calls = [], states = [];
  const loader = createSuggestionScheduler({delay: 0, changed: state => states.push(state), load: async query => {
    calls.push(query);
    return [{name: query + '之下'}];
  }});
  loader.schedule('永'); loader.schedule('永冬'); loader.schedule(' 永冬之 ');
  assert.equal(calls.length, 0);
  await tick();
  assert.deepEqual(calls, ['永冬之']);
  assert.deepEqual(states.at(-1).items, [{name: '永冬之之下'}]);
  loader.clear();
});

test('changed input aborts the old request and ignores out of order success', async () => {
  const {createSuggestionScheduler} = await modulePromise;
  const calls = [], states = [];
  const loader = createSuggestionScheduler({delay: 0, changed: state => states.push(state), load: (query, signal) => new Promise(resolve => calls.push({query, signal, resolve}))});
  loader.schedule('永'); await tick();
  loader.schedule('重生');
  assert.equal(calls[0].signal.aborted, true);
  assert.deepEqual(states.at(-1).items, []);
  await tick();
  calls[1].resolve([{name: '重生回来'}]); await flush();
  calls[0].resolve([{name: '永世长青'}]); await flush();
  assert.equal(states.at(-1).query, '重生');
  assert.deepEqual(states.at(-1).items, [{name: '重生回来'}]);
  loader.clear();
});

test('dismissing suggestions invalidates both pending timers and late responses', async () => {
  const {createSuggestionScheduler} = await modulePromise;
  let calls = 0, finish, signal, state;
  const loader = createSuggestionScheduler({delay: 0, changed: value => {state = value;}, load: (_, current) => {
    calls++; signal = current;
    return new Promise(resolve => {finish = resolve;});
  }});
  loader.schedule('永'); loader.clear(); await tick();
  assert.equal(calls, 0);
  loader.schedule('永'); await tick(); loader.clear();
  assert.equal(signal.aborted, true);
  finish([{name: '永世长青'}]); await flush();
  assert.deepEqual(state, {query: '', items: [], loading: false});
});

test('cached names and valid empty results expire; failures can be retried', async () => {
  const {createSuggestionScheduler} = await modulePromise;
  let timestamp = 1000, calls = 0, failure = false, state;
  const loader = createSuggestionScheduler({delay: 0, now: () => timestamp, changed: value => {state = value;}, load: async () => {
    calls++;
    if (failure) throw new Error('offline');
    return [];
  }});
  loader.schedule('空结果'); await tick();
  loader.schedule(' 空结果 '); await tick();
  assert.equal(calls, 1);
  timestamp += 60001;
  failure = true;
  loader.schedule('空结果'); await tick();
  assert.equal(calls, 2);
  assert.deepEqual(state.items, []);
  failure = false;
  loader.schedule('空结果'); await tick();
  assert.equal(calls, 3);
  loader.clear();
});

test('invalid queries never request and candidate count remains bounded', async () => {
  const {createSuggestionScheduler} = await modulePromise;
  let calls = 0, state;
  const loader = createSuggestionScheduler({delay: 0, changed: value => {state = value;}, load: async () => {
    calls++;
    return [{name: ''}, {name: '换\n行'}, {name: '永世长青'}, {name: '永世长青'}, ...Array.from({length: 20}, (_, i) => ({name: '永夜' + i}))];
  }});
  for (const query of ['', ' ', '换\n行', '永'.repeat(81)]) {loader.schedule(query); await tick();}
  assert.equal(calls, 0);
  loader.schedule('永'); await tick();
  assert.equal(calls, 1);
  assert.equal(state.items.length, 10);
  assert.deepEqual(state.items.slice(0, 2), [{name: '永世长青'}, {name: '永夜0'}]);
  loader.clear();
});

test('the browser cache evicts older queries instead of growing indefinitely', async () => {
  const {createSuggestionScheduler} = await modulePromise;
  let calls = 0;
  const loader = createSuggestionScheduler({delay: 0, changed() {}, load: async query => {calls++; return [{name: query}];}});
  for (let i = 0; i < 33; i++) {loader.schedule('永' + i); await tick();}
  loader.schedule('永0'); await tick();
  assert.equal(calls, 34);
  loader.clear();
});

test('highlighting preserves literal text and complete Unicode characters', async () => {
  const {suggestionParts} = await modulePromise;
  const name = '永😀<img src=x>永';
  const parts = suggestionParts(name, '永😀');
  assert.equal(parts.map(part => part.text).join(''), name);
  assert.deepEqual(parts, [{text: '永😀', highlighted: true}, {text: '<img src=x>', highlighted: false}, {text: '永', highlighted: true}]);
});
