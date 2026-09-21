const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const source = fs.readFileSync(path.join(__dirname, 'search-stream.js'), 'utf8');
const modulePromise = import('data:text/javascript;base64,' + Buffer.from(source).toString('base64'));
const headers = {'Content-Type': 'application/x-ndjson; charset=utf-8'};
const frame = (title, done = false) => ({query: '修仙', source: 'hongguo', data: [{id: 'hongguo:7300000000000000001', title}], done});

test('short batches appear before completion and preserve split UTF-8 text', async () => {
  const {readSearchResponse} = await modulePromise;
  let controller;
  const body = new ReadableStream({start(value) {controller = value;}});
  const seen = [];
  const pending = readSearchResponse(new Response(body, {headers}), result => seen.push(result));
  const first = frame('谁说没灵根不能修仙的？之无灵证道第七季');
  const bytes = new TextEncoder().encode(JSON.stringify(first) + '\n');
  for (const byte of bytes) controller.enqueue(Uint8Array.of(byte));
  await new Promise(resolve => setImmediate(resolve));
  assert.deepEqual(seen, [first], 'a batch below 50 must be delivered while later data is pending');
  const last = frame('第二十季', true);
  controller.enqueue(new TextEncoder().encode(JSON.stringify(last) + '\n'));
  assert.deepEqual(await pending, last);
  assert.deepEqual(seen, [first, last]);
});

test('legacy JSON and browsers without response readers still receive a completed result', async () => {
  const {readSearchResponse} = await modulePromise;
  const data = frame('普通结果');
  delete data.done;
  const seen = [];
  const result = await readSearchResponse(Response.json(data), value => seen.push(value));
  assert.equal(result.done, true);
  assert.deepEqual(seen, [{...data, done: true}]);
  const buffered = {...data, done: true};
  const response = {ok: true, headers: new Headers(headers), text: async () => '\n' + JSON.stringify(buffered)};
  assert.deepEqual(await readSearchResponse(response, () => {}), buffered);
});

test('truncated and malformed streams retain earlier batches and report failure', async () => {
  const {readSearchResponse} = await modulePromise;
  for (const tail of ['', '{broken}\n']) {
    const first = frame('已收到的第七季');
    const response = new Response(JSON.stringify(first) + '\n' + tail, {headers});
    const seen = [];
    await assert.rejects(readSearchResponse(response, value => seen.push(value)), tail ? SyntaxError : /连接中断/);
    assert.deepEqual(seen, [first]);
  }
});

test('upstream errors after a batch and authentication errors are not empty successes', async () => {
  const {readSearchResponse} = await modulePromise;
  const first = frame('已收到的剧集');
  const error = {error: '后续检索失败', done: true};
  const seen = [], checked = [];
  await assert.rejects(readSearchResponse(new Response(JSON.stringify(first) + '\n' + JSON.stringify(error) + '\n', {headers}), value => seen.push(value), value => checked.push(value)), /后续检索失败/);
  assert.deepEqual(seen, [first]);
  assert.deepEqual(checked, [first, error]);
  await assert.rejects(readSearchResponse(Response.json({error: '需要登录'}, {status: 401}), () => assert.fail('unauthorized batch was displayed')), error => error.status === 401 && /需要登录/.test(error.message));
});

test('consumer cancellation stops the response reader', async () => {
  const {readSearchResponse} = await modulePromise;
  let canceled = false;
  const body = new ReadableStream({
    start(controller) {controller.enqueue(new TextEncoder().encode(JSON.stringify(frame('过期搜索')) + '\n'));},
    cancel() {canceled = true;}
  });
  await assert.rejects(readSearchResponse(new Response(body, {headers}), () => {throw new Error('query changed');}), /query changed/);
  assert.equal(canceled, true);
});
