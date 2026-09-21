const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const librarySort = require('./library-sort.js');
const source = fs.readFileSync(path.join(__dirname, 'library-search.js'), 'utf8');
const modulePromise = import('data:text/javascript;base64,' + Buffer.from(source).toString('base64'));

test('title filtering ignores punctuation, spacing, case and character width', async () => {
  const {matchesSearch, normalizeSearchText} = await modulePromise;
  assert.equal(matchesSearch('普通弓箭手？我能无限叠加攻击力第五季', '普通弓箭手我能无限叠加攻击力'), true);
  assert.equal(matchesSearch('普通弓箭手？我能无限叠加攻击力第五季', '普通弓箭手 第五季'), true);
  assert.equal(matchesSearch('ＡＢＣ！无限　叠加攻击', 'abc无限叠加攻击'), true);
  assert.equal(matchesSearch('普通弓箭手第二季', '普通弓箭手 第五季'), false);
  assert.equal(matchesSearch('普通弓箭手', '？！'), false);
  assert.equal(normalizeSearchText('C++'), 'c++');
});

test('all five seasons precede description-only and broadly related online results', async () => {
  const {rankSearchResults} = await modulePromise;
  const base = '普通弓箭手？我能无限叠加攻击力';
  const dramas = [
    {id: 'online', title: '其他推荐'},
    {id: 'description', title: '简介匹配', desc: '无限叠加攻击'},
    ...['第五季', '第三季', '', '第四季', '第二季'].map(suffix => ({id: suffix || '第一季', title: base + suffix}))
  ];
  const getText = drama => [drama.title, drama.desc].join(' ');
  const ranked = rankSearchResults(librarySort.sortDramas(dramas, 'title'), '无限叠加攻击', getText, new Set(['online']));
  assert.deepEqual(ranked.map(drama => drama.id), ['第一季', '第二季', '第三季', '第四季', '第五季', 'description', 'online']);
  assert.equal(dramas[0].id, 'online', 'ranking must not reorder the library');
});

test('complete title and separate search terms take priority while duplicate titles remain', async () => {
  const {rankSearchResults} = await modulePromise;
  const list = [
    {id: 'related', title: '相关作品', desc: '弓箭手 第五季'},
    {id: 'season', title: '弓箭手无限叠加攻击第五季'},
    {id: 'exact1', title: '弓箭手第五季'},
    {id: 'exact2', title: '弓箭手第五季'}
  ];
  const getText = drama => [drama.title, drama.desc].join(' ');
  assert.deepEqual(rankSearchResults(list, '弓箭手 第五季', getText).map(drama => drama.id), ['exact1', 'exact2', 'season', 'related']);
  assert.deepEqual(rankSearchResults(list, '', getText), list);
});
