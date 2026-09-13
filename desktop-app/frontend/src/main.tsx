import React, { useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './style.css';

const OFFICIAL = 'https://alice.guyufl.us.ci';
const CONFIG_KEY = 'alice.relay.url';
function savedServer() { try { return localStorage.getItem(CONFIG_KEY) || OFFICIAL; } catch { return OFFICIAL; } }
function App() {
  const [active] = useState(savedServer);
  const [draft, setDraft] = useState(active);
  const [settings, setSettings] = useState(false);
  const [editing, setEditing] = useState(false);
  const [pending, setPending] = useState(false);
  const [ready, setReady] = useState(false);
  const [status, setStatus] = useState('正在初始化');
  const [code, setCode] = useState('——');
  const [notice, setNotice] = useState('');
  const api = (window as any).go?.main?.App;
  useEffect(() => {
    let mounted = true;
    if (!api) { setStatus('请在桌面应用中使用'); return; }
    api.SetRelayURL(active).then(() => { if(mounted) { setReady(true); setStatus('未连接'); } }).catch(() => { if(mounted) setStatus('服务器配置加载失败'); });
    return () => { mounted = false; };
  }, []);
  const connect = async () => { try { await api.CheckRelayHealth(); setStatus('连接正常'); } catch { setStatus('连接失败，请检查网络或服务器设置'); } };
  const pair = async () => { try { setCode(String(await api.GeneratePairingCode())); setStatus('配对码已获取'); } catch { setStatus('配对码获取失败'); } };
  const test = async () => { try { await api.PublishTestAlert(); setNotice('测试告警已发送'); } catch { setNotice('告警发送失败'); } };
  const save = (value: string) => {
    try {
      const u = new URL(value.trim());
      if (u.protocol !== 'https:' && !(u.protocol === 'http:' && ['localhost','127.0.0.1','[::1]'].includes(u.hostname))) throw new Error();
      if (u.username || u.password || u.search || u.hash || (u.pathname !== '/' && u.pathname !== '')) throw new Error();
      const normalized = u.origin;
      if (normalized === OFFICIAL) localStorage.removeItem(CONFIG_KEY); else localStorage.setItem(CONFIG_KEY, normalized);
      setDraft(normalized); setPending(normalized !== active); setNotice(normalized === active ? '配置未变化' : '已保存，重载应用后生效。当前仍使用原服务器。');
    } catch { setNotice('保存失败：请输入有效 HTTPS 服务器地址（本机调试允许 HTTP），不含路径、账号、查询参数。'); }
  };
  return <main className="app"><header><div><div className="eyebrow">EVE INTEL DESK</div><h1>情报助手 <b>v0.0.1-alpha</b></h1></div><button onClick={() => setSettings(!settings)}>{settings ? '返回首页' : '设置'}</button></header>
    {notice && <div className="panel" role="status">{notice}</div>}
    {pending && <section className="panel"><strong>服务器设置待生效</strong><p>重载会清除当前页面状态；更换服务器后可能需要重新配对。</p><button onClick={() => window.location.reload()}>立即重载应用</button></section>}
    {settings ? <section className="panel"><h2>服务器设置</h2><p>当前使用：{active === OFFICIAL ? 'Alice官方服务器' : '自定义服务器'}</p><p>默认无需修改。更换服务器会改变配对和告警数据的接收方，请仅使用可信服务。</p><button onClick={() => setEditing(!editing)}>{editing ? '收起修改入口' : '修改服务器（高级）'}</button>{editing && <><label htmlFor="server-url">自定义服务器地址</label><div className="row"><input id="server-url" value={draft} onChange={e => setDraft(e.target.value)} /><button onClick={() => save(draft)}>保存，重载后生效</button></div><button className="secondary" onClick={() => save(OFFICIAL)}>恢复Alice官方服务器</button></>}</section> : <><section className="panel"><h2>{active === OFFICIAL ? 'Alice官方服务器' : '自定义服务器'}</h2><p>{status}</p><button disabled={!ready || pending} onClick={connect}>检查连接</button><small>默认连接官方服务，服务器修改入口位于设置。</small></section><section className="grid"><article><h2>移动端配对</h2><p>在移动端输入服务器签发的配对码。</p><div className="code">{code}</div><button disabled={!ready || pending} onClick={pair}>获取配对码</button></article><article><h2>测试告警</h2><button disabled={!ready || pending} onClick={test}>发送测试告警</button></article><article><h2>当前状态</h2><p>{status}</p><p>情报管线尚未接入</p></article></section></>}
  </main>;
}
createRoot(document.getElementById('root')!).render(<App />);
