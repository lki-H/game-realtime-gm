import React, { useEffect, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './style.css';

const views = [['parties', '好友房间'], ['proposals', '匹配确认'], ['runs', '共同作战'], ['tasks', '个人任务'], ['pending', '待处理记录']] as const;
type Row = Record<string, unknown>;
async function readResponse(response: Response) {
 const body=await response.text();
 if (!body) throw new Error(`服务暂不可用（HTTP ${response.status}），请稍后刷新`);
 try { return JSON.parse(body); } catch { throw new Error(`服务返回异常响应（HTTP ${response.status}），请检查后端状态`); }
}
function App() {
 const [token, setToken] = useState('');
 const currentToken = useRef(token); currentToken.current = token;
 const [username, setUsername] = useState('');
 const [password, setPassword] = useState('');
 const [view, setView] = useState<string>('runs');
 const [items, setItems] = useState<Row[]>([]);
 const [error, setError] = useState('');
 const [loading, setLoading] = useState(false);
 const [updated, setUpdated] = useState('');
 const [refresh, setRefresh] = useState(0);
 const [page, setPage] = useState(1);
 const [total, setTotal] = useState(0);
 const [role, setRole] = useState('');
 const [requestId, setRequestId] = useState('');
 const [repair, setRepair] = useState<Row | null>(null);
 const [reason, setReason] = useState('');
 const [repairId, setRepairId] = useState('');
 const [notice, setNotice] = useState('');
 function logout() { currentToken.current=''; setToken(''); setRole(''); setItems([]); setRepair(null); setReason(''); setNotice(''); setTotal(0); setUpdated(''); setRequestId(''); setLoading(false); setPage(1); }
 useEffect(() => {
  if (!token) return;
  const controller = new AbortController();
  setLoading(true); setError('');
  fetch(`/api/admin/v2/observations/${view}?page=${page}&page_size=20`, { headers: { Authorization: `Bearer ${token}` }, signal: AbortSignal.any([controller.signal,AbortSignal.timeout(10000)]) })
   .then(async response => { if(controller.signal.aborted || currentToken.current!==token)return; setRequestId(response.headers.get('X-Request-ID') || ''); if (response.status === 401 || response.status === 403) { logout(); throw new Error('管理员会话已失效，请重新登录'); } const result = await readResponse(response); if(controller.signal.aborted || currentToken.current!==token)return; if (!response.ok || result.code !== 0) throw new Error(result.message || '请求失败'); setTotal(result.pagination?.total || 0); return result.data as Row[]; })
   .then(data => { if(data && !controller.signal.aborted && currentToken.current===token){setItems(data); setUpdated(new Date().toLocaleTimeString());} })
   .catch(failure => { if (!controller.signal.aborted && failure.name !== 'AbortError') {setItems([]); setTotal(0); setUpdated(''); setError(failure.message);} })
   .finally(() => { if (!controller.signal.aborted) setLoading(false); });
  return () => controller.abort();
 }, [token, view, refresh, page]);
 async function login(event: React.FormEvent) {
  event.preventDefault(); setError(''); setLoading(true);
  try {
   const response = await fetch('/api/admin/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password }), signal:AbortSignal.timeout(10000) });
   const result = await readResponse(response); if (!response.ok || result.code !== 0) throw new Error(result.message || '登录失败'); setToken(result.data.token); setRole(result.data.admin.role); setPassword(''); setPage(1);
  } catch (failure) { setError(failure instanceof Error ? failure.message : '登录失败'); } finally { setLoading(false); }
 }
 async function retry(event: React.FormEvent) {
  event.preventDefault(); if (!repair || loading) return; setLoading(true); setError('');
  try {
   const response = await fetch(`/api/admin/v2/operations/${encodeURIComponent(String(repair.operation_id))}/retry`, {method:'POST',headers:{Authorization:`Bearer ${token}`,'Content-Type':'application/json'},body:JSON.stringify({operation_id:repairId,expected_attempts:Number(repair.attempts),reason}),signal:AbortSignal.timeout(10000)});
   if(currentToken.current!==token)return;
   setRequestId(response.headers.get('X-Request-ID') || ''); const result = await readResponse(response);
   if(currentToken.current!==token)return;
   if (response.status === 401 || response.status === 403) logout();
   if (!response.ok || result.code !== 0) throw new Error(result.message || '重试失败；刷新后核对状态与数据修复情况');
   setNotice(`已提交重试：${result.data.operation_id}`); setRepair(null); setReason(''); setRefresh(value=>value+1);
  } catch(failure) {if(currentToken.current===token)setError(failure instanceof Error?failure.message:'重试失败');} finally {if(currentToken.current===token)setLoading(false);}
 }
 const columns = items.length ? Object.keys(items[0]) : [];
 return <main><header><div><p className="eyebrow">COOPERATIVE PVE · V2</p><h1>作战管理观察窗</h1><p className="subtle">房间、匹配、作战与任务处理状态</p></div>{token && <button onClick={logout}>退出登录</button>}</header>
  {error && <p role="alert" className="error">{error}</p>}
  {notice && <p role="status">{notice}</p>}
  {!token ? <form onSubmit={login}><h2>管理员登录</h2><label>账号<input required value={username} onChange={event => setUsername(event.target.value)} autoComplete="username"/></label><label>密码<input required type="password" value={password} onChange={event => setPassword(event.target.value)} autoComplete="current-password"/></label><button disabled={loading} type="submit">{loading ? '登录中…' : '登录'}</button></form> : <><nav aria-label="观察对象">{views.map(([key, name]) => <button key={key} aria-pressed={view === key} onClick={() => { setView(key); setItems([]); setPage(1); setRepair(null); }}>{name}</button>)}</nav><section><div className="toolbar"><h2>{views.find(([key]) => key === view)?.[1]}</h2><span className="subtle">{updated ? `更新于 ${updated}` : ''}</span><button disabled={loading} onClick={() => setRefresh(value => value + 1)}>刷新</button></div>{loading ? <p role="status">正在查询…</p> : items.length ? <div className="table-scroll"><table><thead><tr>{columns.map(column => <th key={column}>{column}</th>)}{view==='pending' && <th>处理</th>}</tr></thead><tbody>{items.map((item, index) => <tr key={String(item.operation_id || item.id || index)}>{columns.map(column => <td key={column}>{typeof item[column] === 'object' ? JSON.stringify(item[column]) : String(item[column] ?? '—')}</td>)}{view==='pending' && <td>{role==='operator' && item.status==='needs_repair' && (item.operation_type==='settlement'||item.operation_type==='task_completion') ? <button disabled={loading} onClick={()=>{setRepair(item);setRepairId(crypto.randomUUID());setReason('');setNotice('');}}>检查并重试</button> : <span>{item.status==='needs_repair'?'需要 operator 修复':'—'}</span>}</td>}</tr>)}</tbody></table></div> : <p className="empty">暂无记录</p>}<div className="toolbar"><button disabled={loading||page===1} onClick={()=>setPage(value=>value-1)}>上一页</button><span>第 {page} 页 · 共 {total} 条</span><button disabled={loading||page*20>=total} onClick={()=>setPage(value=>value+1)}>下一页</button></div>{requestId && <p className="subtle">请求编号：{requestId}</p>}</section>{repair && <form onSubmit={retry}><h2>确认重新尝试</h2><p>记录：{String(repair.operation_id)}，当前尝试次数：{String(repair.attempts)}</p><p>先修复原故障，再重新尝试。服务端会复核状态、版本及当前权限，并记录审计。</p><label>处理原因<textarea required maxLength={256} value={reason} onChange={event=>setReason(event.target.value)}/></label><button disabled={loading||!reason.trim()} type="submit">确认提交重试</button><button type="button" disabled={loading} onClick={()=>setRepair(null)}>取消</button></form>}</>}
 </main>;
}
createRoot(document.getElementById('root')!).render(<App/>);
