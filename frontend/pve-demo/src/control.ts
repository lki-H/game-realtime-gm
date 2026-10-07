export interface Envelope { type?: string; request_id?: string; code?: number; message?: string; data?: unknown }

export class Control {
  token = '';
  socket: WebSocket | null = null;
  changed: (message: Envelope) => void = () => {};
  connection: (connected: boolean) => void = () => {};
  private pending = new Map<string, { resolve: (value: unknown) => void; reject: (error: Error) => void; timer: ReturnType<typeof setTimeout>; intent:{type:string;data:unknown;operationId:string} }>();
  lastUnknown: { type: string; data: unknown; operationId: string } | null = null;

  private async response(response: Response) {
    let result;
    try { result = await response.json(); } catch { throw new Error(`服务响应无效（HTTP ${response.status}），请检查后端与代理`); }
    if (!response.ok || result.code !== 0) throw new Error(result.message || `请求失败（HTTP ${response.status}）`);
    return result;
  }

  async query<T>(path: string): Promise<T> {
    if (!path.startsWith('/api/')) throw new Error('查询地址无效');
    const response = await fetch(path, { headers: { Authorization: `Bearer ${this.token}` }, signal: AbortSignal.timeout(10000) });
    const result = await this.response(response);
    return result.data as T;
  }
  async login(username: string, password: string): Promise<number> {
    const response = await fetch('/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password }), signal: AbortSignal.timeout(10000) });
    const result = await this.response(response);
    this.close();
    this.token = result.data.token;
    this.connect();
    return result.data.player.id;
  }
  connect() {
    if (!this.token) throw new Error('请先登录');
    const previous = this.socket;
    this.socket = null;
    previous?.close();
    this.rejectPending();
    this.connection(false);
    const socket = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws?token=${encodeURIComponent(this.token)}`);
    this.socket = socket;
    const deadline = setTimeout(() => { if (this.socket === socket && socket.readyState !== WebSocket.OPEN) socket.close(); }, 10000);
    socket.onopen = () => { clearTimeout(deadline); if (this.socket === socket) this.connection(true); };
    socket.onclose = () => { clearTimeout(deadline); if (this.socket === socket) { this.socket = null; this.connection(false); this.rejectPending(); } };
    socket.onmessage = event => {
      if (this.socket !== socket || typeof event.data !== 'string' || event.data.length > 1048576) return;
      let message: Envelope;
      try { message = JSON.parse(event.data) as Envelope; } catch { return; }
      const pending = message.request_id ? this.pending.get(message.request_id) : undefined;
      if (pending && message.request_id) {
        clearTimeout(pending.timer); this.pending.delete(message.request_id);
        if (message.code === 0) pending.resolve(message.data); else pending.reject(new Error(message.message || '操作失败'));
      }
      this.changed(message);
    };
  }
  async send<T>(type: string, data: unknown, operationId: string = crypto.randomUUID()): Promise<T> {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) throw new Error('连接未就绪，请重连');
    if (this.pending.size >= 32) throw new Error('等待中的操作过多');
    const requestId = crypto.randomUUID();
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(requestId); this.lastUnknown = { type, data, operationId };
        reject(new Error('操作结果尚不确定。请恢复状态或重试同一操作。'));
      }, 15000);
      this.pending.set(requestId, { resolve: value => { if(this.lastUnknown?.operationId === operationId)this.lastUnknown = null; resolve(value as T); }, reject, timer,intent:{type,data,operationId} });
      try {
        this.socket!.send(JSON.stringify({ schema_version: 2, type, data, operation_id: operationId, request_id: requestId }));
      } catch {
        clearTimeout(timer); this.pending.delete(requestId); reject(new Error('操作未能发送，请重连后恢复状态'));
      }
    });
  }
  close() { const socket = this.socket; this.socket = null; socket?.close(); this.token = ''; this.rejectPending(); this.lastUnknown = null; this.connection(false); }
  private rejectPending() { for (const pending of this.pending.values()) { clearTimeout(pending.timer); this.lastUnknown=pending.intent; pending.reject(new Error('连接已关闭，请查询恢复状态')); } this.pending.clear(); }
}
