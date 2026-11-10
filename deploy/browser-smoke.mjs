// Real-browser acceptance for disposable Linux CI storage. The caller owns the
// backend/frontends and their cleanup. This script never starts or resets them.
import { randomBytes, randomUUID } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import path from 'node:path';

const report = {
  schema_version: 1,
  status: 'RUNNING',
  scope: 'real isolated backend and actual frontend browser smoke',
  started_at: new Date().toISOString(),
  checks: [],
  mock_responses: false,
  gameplay_started: false,
  browser_closed: false,
  storage_cleanup: 'CALLER_OWNED',
};
let phase = 'configuration', browser, context, page;
let playerToken = '', adminToken = '', playerID = 0;
let ownedPartyID = '', pageErrors = 0;
const websocket = { opened: 0, welcomes: 0, closed: 0, decode_errors: 0, sent: [], received: [] };

// Playwright's debug logger can include filled form values. Keep this process's
// diagnostic output disabled and emit only our explicit, credential-free report.
process.env.DEBUG = '';
process.env.PWDEBUG = '0';

class SmokeFailure extends Error {
  constructor(code) { super(code); this.code = code; }
}
function requireValue(name) {
  const value = process.env[name];
  if (!value || /[\r\n\0]/.test(value)) throw new SmokeFailure(`invalid_${name.toLowerCase()}`);
  return value;
}
function localOrigin(name, fallback) {
  let value;
  try { value = new URL(process.env[name] || fallback); } catch { throw new SmokeFailure(`invalid_${name.toLowerCase()}`); }
  if (value.protocol !== 'http:' || !['127.0.0.1', 'localhost', '[::1]'].includes(value.hostname) || value.username || value.password || value.pathname !== '/' || value.search || value.hash) {
    throw new SmokeFailure(`invalid_${name.toLowerCase()}`);
  }
  return value.origin;
}
function check(condition, code) {
  if (!condition) throw new SmokeFailure(code);
}
function passed(name, details = {}) { report.checks.push({ name, status: 'PASS', ...details }); }
async function until(condition, code, timeout = 15000) {
  const expires = Date.now() + timeout;
  while (Date.now() < expires) {
    if (await condition()) return;
    await new Promise(resolve => setTimeout(resolve, 50));
  }
  throw new SmokeFailure(code);
}
function responseMatches(response, pathname, method = 'GET') {
  return new URL(response.url()).pathname === pathname && response.request().method() === method;
}
function waitResponse(pathname, method = 'GET') {
  const pending = page.waitForResponse(response => responseMatches(response, pathname, method));
  // Several responses begin concurrently during login. Keep an abandoned wait
  // handled if another assertion fails before this promise is awaited.
  pending.catch(() => {});
  return pending;
}
async function responseJSON(response, code) {
  let value;
  try { value = await response.json(); } catch { throw new SmokeFailure(code); }
  check(value && typeof value === 'object' && !Array.isArray(value), code);
  return value;
}
async function api(base, pathname, { method = 'GET', token = '', data } = {}) {
  const headers = { Accept: 'application/json' };
  if (token) headers.Authorization = `Bearer ${token}`;
  if (data !== undefined) headers['Content-Type'] = 'application/json';
  const response = await fetch(new URL(pathname, base), {
    method, headers, body: data === undefined ? undefined : JSON.stringify(data),
    redirect: 'error', signal: AbortSignal.timeout(10000),
  });
  let value;
  try { value = await response.json(); } catch { throw new SmokeFailure('invalid_api_json'); }
  check(value && typeof value === 'object' && !Array.isArray(value), 'invalid_api_envelope');
  return { http: response.status, value };
}
async function activity(base) {
  const result = await api(base, '/api/v2/me/activity', { token: playerToken });
  check(result.http === 200 && result.value.code === 0 && result.value.data && typeof result.value.data === 'object', 'activity_query_failed');
  return result.value.data;
}
async function command(type, action) {
  const beforeSent = websocket.sent.length, beforeReceived = websocket.received.length;
  await action();
  await until(() => websocket.sent.slice(beforeSent).some(frame => frame.type === type), 'command_was_not_sent');
  const frame = websocket.sent.slice(beforeSent).find(frame => frame.type === type);
  await until(() => websocket.received.slice(beforeReceived).some(result => result.request_id === frame.request_id && result.type === `${type}.result`), 'command_receipt_missing');
  const receipt = websocket.received.slice(beforeReceived).find(result => result.request_id === frame.request_id && result.type === `${type}.result`);
  check(receipt.code === 0, 'command_rejected');
  passed(type);
}
function observePage(target) {
  target.on('pageerror', () => { pageErrors++; });
  target.on('websocket', socket => {
    if (new URL(socket.url()).pathname !== '/ws') return;
    websocket.opened++;
    socket.on('close', () => { websocket.closed++; });
    for (const event of ['framesent', 'framereceived']) {
      socket.on(event, frame => {
        let value;
        try { value = JSON.parse(String(frame.payload)); } catch { websocket.decode_errors++; return; }
        if (!value || typeof value !== 'object' || Array.isArray(value)) { websocket.decode_errors++; return; }
        // Keep correlation metadata only. Never retain frame data, token URLs,
        // login requests, or server-provided message text in diagnostics.
        const safe = { type: value.type, request_id: value.request_id, code: value.code };
        if (event === 'framesent') websocket.sent.push(safe);
        else {
          websocket.received.push(safe);
          if (value.type === 'server.welcome') websocket.welcomes++;
        }
      });
    }
  });
}

async function run() {
  check(process.env.GM_BROWSER_SMOKE === '1', 'disposable_browser_opt_in_required');
  const base = localOrigin('GM_SMOKE_BASE_URL', 'http://127.0.0.1:18080');
  const adminURL = localOrigin('GM_SMOKE_ADMIN_URL', 'http://127.0.0.1:5173');
  const playerURL = localOrigin('GM_SMOKE_PVE_URL', 'http://127.0.0.1:5174');
  check(new URL(playerURL).port === '5174' && ['127.0.0.1', 'localhost'].includes(new URL(playerURL).hostname), 'pve_websocket_origin_must_use_5174');
  const adminUsername = requireValue('GM_SMOKE_ADMIN_USERNAME');
  const adminPassword = requireValue('GM_SMOKE_ADMIN_PASSWORD');
  const moduleRoot = requireValue('GM_BROWSER_MODULE_ROOT');
  check(path.isAbsolute(moduleRoot), 'browser_module_root_must_be_absolute');
  const playwright = createRequire(path.join(moduleRoot, 'package.json'))('playwright');
  const playerUsername = `browser_${randomUUID().replaceAll('-', '')}`;
  const playerPassword = randomBytes(32).toString('hex');

  phase = 'readiness_and_registration';
  const readiness = await fetch(new URL('/ready', base), { redirect: 'error', signal: AbortSignal.timeout(10000) });
  check(readiness.status === 200, 'backend_not_ready');
  const registration = await api(base, '/api/register', { method: 'POST', data: { username: playerUsername, password: playerPassword, nickname: 'Browser smoke' } });
  check(registration.http === 201 && registration.value.code === 0 && Number.isSafeInteger(registration.value.data?.id) && registration.value.data.id > 0, 'player_registration_failed');
  playerID = registration.value.data.id;
  passed('disposable_player_registration', { http: registration.http });

  phase = 'browser_launch';
  browser = await playwright.chromium.launch({ headless: true, timeout: 20000 });
  context = await browser.newContext({ viewport: { width: 1280, height: 900 }, locale: 'zh-CN' });
  context.setDefaultTimeout(15000);
  page = await context.newPage();
  observePage(page);

  phase = 'admin_login';
  await page.goto(adminURL, { waitUntil: 'domcontentloaded' });
  await page.getByRole('heading', { name: '管理员登录', exact: true }).waitFor();
  await page.getByLabel('账号', { exact: true }).fill(adminUsername);
  await page.getByLabel('密码', { exact: true }).fill(randomBytes(32).toString('hex'));
  const rejectedLogin = waitResponse('/api/admin/login', 'POST');
  await page.getByRole('button', { name: '登录', exact: true }).click();
  const rejectedResponse = await rejectedLogin, rejectedBody = await responseJSON(rejectedResponse, 'admin_rejection_not_json');
  check(rejectedResponse.status() === 401 && rejectedBody.code === 40111, 'admin_bad_password_not_rejected');
  await page.getByRole('alert').waitFor();
  await page.getByRole('heading', { name: '管理员登录', exact: true }).waitFor();
  passed('admin_wrong_password_rejected', { http: 401, code: 40111 });
  await page.getByLabel('密码', { exact: true }).fill(adminPassword);
  const adminLogin = waitResponse('/api/admin/login', 'POST');
  const initialRuns = waitResponse('/api/admin/v2/observations/runs');
  await page.getByRole('button', { name: '登录', exact: true }).click();
  const adminResponse = await adminLogin, adminBody = await responseJSON(adminResponse, 'admin_login_not_json');
  check(adminResponse.status() === 200 && adminBody.code === 0 && typeof adminBody.data?.token === 'string', 'admin_login_failed');
  adminToken = adminBody.data.token;
  const role = adminBody.data.admin?.role;
  check(role === 'gm' || role === 'operator', 'unsupported_smoke_admin_role');
  const initialResponse = await initialRuns, initialBody = await responseJSON(initialResponse, 'initial_observation_not_json');
  check(initialResponse.status() === 200 && initialBody.code === 0 && Array.isArray(initialBody.data), 'initial_observation_failed');
  await page.getByRole('heading', { name: '共同作战', exact: true }).waitFor();
  await until(() => page.getByRole('button', { name: '刷新', exact: true }).isEnabled(), 'admin_query_still_busy');
  passed('admin_valid_login');

  phase = 'admin_observations';
  for (const [entity, label] of [['parties', '好友房间'], ['proposals', '匹配确认'], ['runs', '共同作战'], ['tasks', '个人任务'], ['pending', '待处理记录'], ['outbox', '通知记录']]) {
    const queried = waitResponse(`/api/admin/v2/observations/${entity}`);
    await page.getByRole('navigation', { name: '观察对象' }).getByRole('button', { name: label, exact: true }).click();
    const response = await queried, value = await responseJSON(response, 'observation_not_json');
    check(response.status() === 200 && value.code === 0 && Array.isArray(value.data), 'observation_failed');
    check(Number.isSafeInteger(value.pagination?.total) && value.pagination.total >= 0, 'observation_pagination_invalid');
    await page.getByRole('heading', { name: label, exact: true }).waitFor();
    await until(() => page.getByRole('button', { name: '刷新', exact: true }).isEnabled(), 'observation_still_busy');
    check(await page.getByRole('button', { name: '上一页', exact: true }).isDisabled(), 'first_page_previous_not_disabled');
    passed(`admin_observation_${entity}`, { rows: value.data.length });
  }
  const refreshed = waitResponse('/api/admin/v2/observations/outbox');
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  check((await refreshed).status() === 200, 'admin_refresh_failed');
  await page.getByRole('button', { name: '退出登录', exact: true }).click();
  await page.getByRole('heading', { name: '管理员登录', exact: true }).waitFor();
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.getByRole('heading', { name: '管理员登录', exact: true }).waitFor();
  check(await page.getByLabel('密码', { exact: true }).inputValue() === '', 'admin_password_persisted');
  passed('admin_logout_and_reload');

  phase = 'player_login';
  // Observe native sockets solely to close this test page's current connection.
  // Constructor/protocol/messages remain real; no HTTP/WS responses are mocked.
  await page.addInitScript(() => {
    const sockets = new Set(), NativeSocket = globalThis.WebSocket;
    globalThis.WebSocket = new Proxy(NativeSocket, {
      construct(target, args, newTarget) {
        const socket = Reflect.construct(target, args, newTarget);
        sockets.add(socket);
        socket.addEventListener('close', () => sockets.delete(socket));
        return socket;
      },
    });
    Object.defineProperty(globalThis, '__gmSmokeCloseSocket', { value: () => {
      let closed = 0;
      for (const socket of sockets) {
        if (socket.readyState === NativeSocket.OPEN && new URL(socket.url).pathname === '/ws') { socket.close(1000, 'browser smoke reconnect'); closed++; }
      }
      return closed;
    } });
  });
  await page.goto(playerURL, { waitUntil: 'domcontentloaded' });
  await page.getByRole('heading', { name: '进入作战大厅', exact: true }).waitFor();
  await page.getByLabel('玩家账号', { exact: true }).fill(playerUsername);
  await page.getByLabel('密码', { exact: true }).fill(randomBytes(32).toString('hex'));
  const wrongPlayerLogin = waitResponse('/api/login', 'POST');
  await page.getByRole('button', { name: '登录并连接', exact: true }).click();
  const wrongPlayerResponse = await wrongPlayerLogin, wrongPlayerBody = await responseJSON(wrongPlayerResponse, 'player_rejection_not_json');
  check(wrongPlayerResponse.status() === 401 && wrongPlayerBody.code === 40101, 'player_bad_password_not_rejected');
  await page.getByRole('alert').waitFor();
  passed('player_wrong_password_rejected', { http: 401, code: 40101 });
  await page.getByLabel('密码', { exact: true }).fill(playerPassword);
  const playerLogin = waitResponse('/api/login', 'POST');
  const tasksQuery = waitResponse('/api/v2/tasks');
  const previewQuery = waitResponse('/api/v2/operations/preview');
  await page.getByRole('button', { name: '登录并连接', exact: true }).click();
  const playerResponse = await playerLogin, playerBody = await responseJSON(playerResponse, 'player_login_not_json');
  check(playerResponse.status() === 200 && playerBody.code === 0 && playerBody.data?.player?.id === playerID && typeof playerBody.data?.token === 'string', 'player_login_failed');
  playerToken = playerBody.data.token;
  await until(() => websocket.welcomes >= 1, 'real_websocket_welcome_missing');
  await until(async () => (await page.locator('.rail-foot').textContent()).includes('已连接作战大厅'), 'player_connection_ui_missing');
  for (const response of await Promise.all([tasksQuery, previewQuery])) {
    const value = await responseJSON(response, 'player_query_not_json');
    check(response.status() === 200 && value.code === 0, 'player_initial_query_failed');
  }
  check(!(await activity(base)).party, 'registered_player_is_not_clean');
  passed('player_real_login_websocket_and_queries');

  phase = 'authorization_probes';
  const unauthenticated = await api(base, '/api/admin/v2/observations/runs');
  check(unauthenticated.http === 401 && unauthenticated.value.code === 40112, 'admin_missing_auth_not_rejected');
  passed('admin_observation_requires_auth', { http: 401, code: 40112 });
  const playerAsAdmin = await api(base, '/api/admin/v2/observations/runs', { token: playerToken });
  check(playerAsAdmin.http === 403 && playerAsAdmin.value.code === 40311, 'player_observation_privilege_leak');
  passed('player_cannot_observe_admin', { http: 403, code: 40311 });
  const rejectedRepair = await api(base, '/api/admin/v2/operations/browser_smoke_missing/retry', {
    method: 'POST', token: adminToken, data: { operation_id: randomUUID(), expected_attempts: 0, reason: 'browser smoke invalid request' },
  });
  const expectedRepair = role === 'operator' ? [400, 40070] : [403, 40370];
  check(rejectedRepair.http === expectedRepair[0] && rejectedRepair.value.code === expectedRepair[1], 'repair_authorization_or_validation_failed');
  passed(role === 'operator' ? 'operator_invalid_repair_rejected' : 'gm_cannot_retry', { http: expectedRepair[0], code: expectedRepair[1] });

  phase = 'room_commands';
  await page.getByRole('navigation', { name: '主要功能' }).getByRole('button', { name: /小队与招募/ }).click();
  await command('v2.party.create', () => page.getByRole('button', { name: '创建好友房间', exact: true }).click());
  await page.getByRole('button', { name: '离开房间', exact: true }).waitFor();
  let snapshot = await activity(base);
  check(snapshot.party?.owner_id === playerID && snapshot.party.members?.length === 1 && snapshot.party.status === 'open', 'created_room_snapshot_invalid');
  ownedPartyID = snapshot.party.id;
  await command('v2.party.plan_update', () => page.getByRole('button', { name: '应用关卡与补位方案', exact: true }).click());
  await command('v2.party.ready', () => page.getByRole('button', { name: '准备', exact: true }).click());
  await page.getByRole('button', { name: '取消准备', exact: true }).waitFor();
  snapshot = await activity(base);
  check(snapshot.party?.id === ownedPartyID && snapshot.party.members.find(member => member.player_id === playerID)?.ready === true, 'ready_snapshot_invalid');
  await command('v2.party.ready', () => page.getByRole('button', { name: '取消准备', exact: true }).click());
  await page.getByRole('button', { name: '准备', exact: true }).waitFor();
  snapshot = await activity(base);
  check(snapshot.party?.members.find(member => member.player_id === playerID)?.ready === false, 'unready_snapshot_invalid');

  phase = 'real_websocket_reconnect';
  const previousWelcomes = websocket.welcomes;
  check(await page.evaluate(() => globalThis.__gmSmokeCloseSocket()) === 1, 'test_connection_not_closed');
  await page.getByRole('button', { name: '重新连接', exact: true }).waitFor();
  await page.getByRole('button', { name: '重新连接', exact: true }).click();
  await until(() => websocket.welcomes > previousWelcomes, 'reconnected_websocket_welcome_missing');
  await until(async () => (await page.locator('.rail-foot').textContent()).includes('已连接作战大厅'), 'reconnected_ui_missing');
  snapshot = await activity(base);
  check(snapshot.party?.id === ownedPartyID, 'reconnect_changed_room_identity');
  passed('real_websocket_close_reconnect_and_room_recovery');
  await command('v2.party.leave', () => page.getByRole('button', { name: '离开房间', exact: true }).click());
  await page.getByRole('button', { name: '创建好友房间', exact: true }).waitFor();
  check(!(await activity(base)).party, 'room_leave_did_not_persist');
  ownedPartyID = '';

  phase = 'logout_and_finish';
  await page.getByRole('button', { name: '退出登录', exact: true }).click();
  await page.getByRole('heading', { name: '进入作战大厅', exact: true }).waitFor();
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.getByRole('heading', { name: '进入作战大厅', exact: true }).waitFor();
  check(await page.getByLabel('密码', { exact: true }).inputValue() === '', 'player_password_persisted');
  passed('player_logout_and_reload');
  check(pageErrors === 0 && websocket.decode_errors === 0, 'browser_or_frame_errors_detected');
  report.status = 'PASS';
}

let deadline;
try {
  await Promise.race([
    run(),
    new Promise((_, reject) => { deadline = setTimeout(() => reject(new SmokeFailure('browser_smoke_deadline_exceeded')), 150000); }),
  ]);
} catch (error) {
  report.status = 'FAIL';
  // Native/Playwright errors can contain a filled value or tokenized URL.
  // Only codes we authored are allowed into public diagnostics.
  report.failure = { phase, code: error instanceof SmokeFailure ? error.code : 'browser_or_transport_failure' };
} finally {
  clearTimeout(deadline);
  if (context) await context.close().catch(() => {});
  if (browser) {
    try { await browser.close(); report.browser_closed = true; } catch { report.browser_closed = false; }
  }
  if (!report.browser_closed && browser) report.status = 'FAIL';
  report.unhandled_page_errors = pageErrors;
  report.websocket = { created: websocket.opened, welcomes: websocket.welcomes, closed: websocket.closed, decode_errors: websocket.decode_errors };
  report.own_room_left = ownedPartyID === '';
  report.completed_at = new Date().toISOString();
  playerToken = ''; adminToken = '';
  const evidence = process.env.GM_SMOKE_EVIDENCE_DIR;
  if (evidence) {
    try {
      check(path.isAbsolute(evidence), 'evidence_directory_must_be_absolute');
      await mkdir(evidence, { recursive: true, mode: 0o700 });
      await writeFile(path.join(evidence, 'browser-summary.json'), `${JSON.stringify(report, null, 2)}\n`, { mode: 0o600 });
    } catch {
      report.status = 'FAIL';
      report.failure = { phase: 'evidence', code: 'browser_evidence_write_failed' };
    }
  }
  process.stdout.write(`${JSON.stringify(report)}\n`);
  process.exitCode = report.status === 'PASS' ? 0 : 1;
}
