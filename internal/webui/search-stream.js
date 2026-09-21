export async function readSearchResponse(response, onResult, checkResponse = () => {}) {
  function accept(data) {
    if (!data || typeof data !== 'object') throw new Error('未收到有效的搜索结果');
    checkResponse(data);
    if (!response.ok || data.error) {
      const error = new Error(data.error || 'HTTP ' + response.status);
      error.status = response.status;
      throw error;
    }
    onResult(data);
    return data;
  }
  if (!response.ok || !response.headers.get('Content-Type')?.includes('application/x-ndjson')) {
    return accept({...await response.json(), done: true});
  }
  let buffer = '', finalResult;
  function consume(text) {
    buffer += text;
    if (buffer.length > 8 * 1024 * 1024) throw new Error('搜索结果过大，请缩小关键词范围');
    let end;
    while ((end = buffer.indexOf('\n')) >= 0) {
      const line = buffer.slice(0, end).trim();
      buffer = buffer.slice(end + 1);
      if (!line) continue;
      if (finalResult) throw new Error('搜索返回了重复的结束标记');
      const data = accept(JSON.parse(line));
      if (data.done === true) finalResult = data;
    }
  }
  if (response.body?.getReader) {
    const reader = response.body.getReader(), decoder = new TextDecoder();
    try {
      while (!finalResult) {
        const {value, done} = await reader.read();
        if (done) {consume(decoder.decode() + '\n'); break;}
        consume(decoder.decode(value, {stream: true}));
      }
    } finally {
      await reader.cancel().catch(() => {});
      reader.releaseLock();
    }
  } else {
    consume(await response.text() + '\n');
  }
  if (!finalResult) throw new Error('搜索连接中断，已保留收到的结果，请重试');
  return finalResult;
}
