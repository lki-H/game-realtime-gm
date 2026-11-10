import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

// Execute the real module with browser transports and time under test control.
// Type checking remains the normal build's responsibility; no files are emitted.
const source = await readFile(new URL('../src/control.ts', import.meta.url), 'utf8');
const compiled = ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 },
}).outputText;
const { Control } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`);

function browser(t) {
  const descriptors = new Map();
  const timers = new Map();
  const sockets = [];
  const controls = [];
  let now = 0, timerID = 0;
  function replace(name, value) {
    if (!descriptors.has(name)) descriptors.set(name, Object.getOwnPropertyDescriptor(globalThis, name));
    Object.defineProperty(globalThis, name, { configurable: true, writable: true, value });
  }
  class Socket {
    static OPEN = 1;
    readyState = 0;
    sent = [];
    failSend = false;
    constructor(url) { this.url = url; sockets.push(this); }
    open() { this.readyState = Socket.OPEN; this.onopen?.({}); }
    close() { this.readyState = 3; this.onclose?.({}); }
    send(value) {
      if (this.failSend) throw new Error('synthetic send failure');
      this.sent.push(JSON.parse(value));
    }
    deliver(value) { this.deliverRaw(JSON.stringify(value)); }
    deliverRaw(value) { this.onmessage?.({ data: value }); }
  }
  replace('WebSocket', Socket);
  replace('location', { protocol: 'http:', host: '127.0.0.1:5174' });
  replace('crypto', { randomUUID });
  replace('setTimeout', (callback, delay) => {
    const id = ++timerID;
    timers.set(id, { at: now + delay, callback });
    return id;
  });
  replace('clearTimeout', id => timers.delete(id));
  t.after(() => {
    for (const control of controls) control.close();
    for (const [name, descriptor] of descriptors) {
      if (descriptor) Object.defineProperty(globalThis, name, descriptor);
      else delete globalThis[name];
    }
  });
  return {
    sockets,
    create() {
      const control = new Control(), changed = [], connections = [];
      controls.push(control);
      control.changed = message => changed.push(message);
      control.connection = connected => connections.push(connected);
      control.token = 'synthetic-browser-token';
      control.connect();
      const socket = sockets.at(-1);
      socket.open();
      return { control, socket, changed, connections };
    },
    advance(milliseconds) {
      now += milliseconds;
      for (;;) {
        const due = [...timers].filter(([, timer]) => timer.at <= now).sort((first, second) => first[1].at - second[1].at)[0];
        if (!due) break;
        timers.delete(due[0]);
        due[1].callback();
      }
    },
  };
}

function receipt(socket, frame = socket.sent.at(-1), code = 0, data = {}) {
  socket.deliver({ type: `${frame.type}.result`, request_id: frame.request_id, code, data, message: code ? 'synthetic rejection' : 'ok' });
}

async function timeout(harness, control, type = 'v2.party.create', operationID = 'operation-original') {
  const pending = control.send(type, {}, operationID);
  const rejected = assert.rejects(pending, /操作结果尚不确定/);
  harness.advance(15000);
  await rejected;
}

test('ordinary success and rejection settle the matching operation', async t => {
  const { control, socket } = browser(t).create();
  const first = control.send('v2.party.create', {}, 'operation-success');
  receipt(socket, undefined, 0, { id: 'party-one' });
  assert.deepEqual(await first, { id: 'party-one' });
  const second = control.send('v2.party.leave', {}, 'operation-rejected');
  const rejected = assert.rejects(second, /synthetic rejection/);
  receipt(socket, undefined, 40970);
  await rejected;
  assert.equal(control.lastUnknown, null);
});

test('malformed envelopes and receipt without a numeric code cannot settle pending work', async t => {
  const { control, socket } = browser(t).create();
  const pending = control.send('v2.party.create', {}, 'operation-shape');
  const frame = socket.sent.at(-1);
  let settled = false;
  pending.then(() => { settled = true; }, () => { settled = true; });
  for (const value of [null, [], 17, true, 'text',
    { request_id: frame.request_id, code: '0' },
    { request_id: frame.request_id, code: 0.5 },
    { request_id: 17, code: 0 },
    { request_id: frame.request_id, type: [] },
    { request_id: frame.request_id, message: {} },
    { request_id: frame.request_id },
  ]) assert.doesNotThrow(() => socket.deliver(value));
  assert.doesNotThrow(() => socket.deliverRaw('{invalid json'));
  await Promise.resolve();
  assert.equal(settled, false);
  receipt(socket, frame, 0, { id: 'shape-safe' });
  assert.deepEqual(await pending, { id: 'shape-safe' });
});

test('a valid late receipt clears unknown work and allows a new operation', async t => {
  const harness = browser(t), { control, socket } = harness.create();
  await timeout(harness, control);
  const frame = socket.sent.at(-1);
  assert.equal(control.lastUnknown.operationId, frame.operation_id);
  await assert.rejects(control.send('v2.party.leave', {}, 'operation-different'), /上一项操作结果尚不确定/);
  receipt(socket, frame);
  assert.equal(control.lastUnknown, null);
  const next = control.send('v2.party.leave', {}, 'operation-after-late');
  receipt(socket);
  await next;
});

test('only a shaped receipt for the original request clears the unknown barrier', async t => {
  const harness = browser(t), { control, socket } = harness.create();
  await timeout(harness, control);
  const frame = socket.sent.at(-1);
  for (const value of [null, [], { request_id: 'another-request', code: 0 },
    { request_id: frame.request_id }, { request_id: frame.request_id, code: '0' },
    { request_id: frame.request_id, code: 0.25 }, { request_id: frame.request_id, message: 42, code: 0 },
  ]) {
    assert.doesNotThrow(() => socket.deliver(value));
    assert.equal(control.lastUnknown.operationId, frame.operation_id);
  }
  receipt(socket, frame, 40970);
  assert.equal(control.lastUnknown, null);
});

test('retry preserves operation identity and uses a fresh request id', async t => {
  const harness = browser(t), { control, socket } = harness.create();
  await timeout(harness, control);
  const original = socket.sent.at(-1), intent = control.lastUnknown;
  const retried = control.send(intent.type, intent.data, intent.operationId);
  const retry = socket.sent.at(-1);
  assert.equal(retry.operation_id, original.operation_id);
  assert.equal(retry.type, original.type);
  assert.deepEqual(retry.data, original.data);
  assert.notEqual(retry.request_id, original.request_id);
  receipt(socket, retry);
  await retried;
  assert.equal(control.lastUnknown, null);
});

test('replaced sockets and old requests cannot clear a newer unknown retry', async t => {
  const harness = browser(t), { control, socket: oldSocket } = harness.create();
  await timeout(harness, control);
  const original = oldSocket.sent.at(-1);
  control.connect();
  const socket = harness.sockets.at(-1);
  socket.open();
  receipt(oldSocket, original);
  assert.equal(control.lastUnknown.operationId, original.operation_id);
  await timeout(harness, control, original.type, original.operation_id);
  const retry = socket.sent.at(-1);
  receipt(socket, original);
  assert.equal(control.lastUnknown.operationId, original.operation_id);
  receipt(oldSocket, retry);
  assert.equal(control.lastUnknown.operationId, original.operation_id);
  receipt(socket, retry);
  assert.equal(control.lastUnknown, null);
});

test('disconnect preserves a pending intent for same-operation recovery; logout clears it', async t => {
  const harness = browser(t), { control, socket, connections } = harness.create();
  const pending = control.send('v2.party.create', {}, 'operation-disconnected');
  const rejected = assert.rejects(pending, /连接已关闭/);
  socket.close();
  await rejected;
  assert.equal(connections.at(-1), false);
  assert.equal(control.lastUnknown.operationId, 'operation-disconnected');
  control.connect();
  const replacement = harness.sockets.at(-1);
  replacement.open();
  const intent = control.lastUnknown, retried = control.send(intent.type, intent.data, intent.operationId);
  receipt(replacement);
  await retried;
  assert.equal(control.lastUnknown, null);
  control.close();
  assert.equal(control.token, '');
  assert.equal(control.socket, null);
  harness.advance(20000);
  assert.equal(control.lastUnknown, null);
});

test('a send failure frees pending work without inventing an unknown operation', async t => {
  const { control, socket } = browser(t).create();
  socket.failSend = true;
  await assert.rejects(control.send('v2.party.create', {}, 'operation-unsent'), /操作未能发送/);
  assert.equal(control.lastUnknown, null);
  socket.failSend = false;
  const pending = control.send('v2.party.create', {}, 'operation-sent');
  receipt(socket);
  await pending;
});
