import React, { useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './style.css';

const OFFICIAL = 'https://alice.guyufl.us.ci';
const KEY = 'alice.relay.url';
const saved = () => { try { return localStorage.getItem(KEY) || OFFICIAL; } catch { return OFFICIAL; } };

type Finding = { kind: string; value: string; stance: string; source: string; summary: string; observedAt: string; expiresAt: string; confidence: number };

function App() {
  const [active] = useState(saved), [ready, setReady] = useState(false), [status, setStatus] = useState('正在初始化');
  const [code, setCode] = useState('——'), [intel, setIntel] = useState(''), [findings, setFindings] = useState<Finding[]>([]);
  const [notice, setNotice] = useState(''), [settings, setSettings] = useState(false);
  const [parsing, setParsing] = useState(false), [parsed, setParsed] = useState(false);
  const api = (window as any).go?.main?.App;
  useEffect(() => { if (!api) { setStatus('请在桌面应用中使用'); return; } api.SetRelayURL(active).then(() => { setReady(true); setStatus('未连接'); }).catch(() => setStatus('服务器配置加载失败')); }, []);
  const connect = async () => { try { await api.CheckRelayHealth(); setStatus('连接正常'); } catch { setStatus('连接失败：请检查网络或服务器设置'); } };
  const pair = async () => { try { setCode(String(await api.GeneratePairingCode())); setStatus('配对码已获取'); } catch { setNotice('配对失败：无法生成配对码'); } };
  const test = async () => { try { await api.PublishTestAlert(); setNotice('测试告警已发送'); } catch { setNotice('测试告警失败：服务未连接或接口不可用'); } };
  const parse = async () => { if (!intel.trim()) { setFindings([]); setParsed(false); setNotice('请输入 Intel 文本后再解析'); return; } setParsing(true); try { const result = await api.ParseIntel(intel); setFindings(result || []); setParsed(true); setNotice(result?.length ? `解析完成：发现 ${result.length} 条确定性结果` : '解析完成：未识别到支持的实体，不代表没有风险'); } catch { setFindings([]); setParsed(false); setNotice('解析失败：桌面后端不可用，请重新启动应用后重试'); } finally { setParsing(false); } };
  return <main className="app"><header><div><div className="eyebrow">EVE INTEL DESK</div><h1>情报工作台 <b>v0.0.1-alpha</b></h1><p>桌面端情报采集、解析与告警测试</p></div><button onClick={() => setSettings(!settings)}>{settings ? '返回工作台' : '设置'}</button></header>
    {notice && <div className="notice" role="status">{notice}</div>}
    {settings ? <section className="panel"><h2>服务器设置</h2><p>当前：{active === OFFICIAL ? 'Alice 官方服务器' : '自定义服务器'}</p><p className="muted">账号授权不在此处处理。服务器地址修改请在配置文件中完成。</p></section> : <>
      <section className="panel connection"><div><h2>连接与配对</h2><p className="muted">默认使用 Alice 官方服务器</p><span className={'pill ' + (status === '连接正常' ? 'ok' : '')}><i /> {status}</span></div><div className="actions"><button disabled={!ready} onClick={connect}>检查连接</button><button disabled={!ready} onClick={pair}>生成配对码</button></div><div className="paircode">{code}</div></section>
      <section className="panel"><h2>Intel 输入</h2><textarea value={intel} onChange={e => setIntel(e.target.value)} placeholder="粘贴频道 Intel 文本，例如：09-20 12:30 — Jita local..." /><div className="toolbar"><span className="muted">解析仅基于输入文本，不调用外部数据</span><button disabled={!api || parsing} onClick={parse}>{parsing ? '解析中…' : '解析 Intel'}</button></div></section>
      <section className="panel results"><h2>解析结果 <span className="count">{findings.length}</span></h2>{parsed && <small className="muted">仅基于本次粘贴文本的规则解析</small>}{findings.length === 0 ? <p className="muted">尚未解析。结果将显示输入中识别到的星系、舰船与立场。</p> : <div className="finding-list">{findings.map((f, i) => <div className="finding" key={`${f.kind}-${f.value}-${i}`}><strong>{f.value}</strong><span>{f.kind} · {f.stance} · 置信度 {(f.confidence * 100).toFixed(0)}%</span><small>{f.summary} · 来源：{f.source}</small></div>)}</div>}</section>
      <section className="grid"><article><h2>测试告警</h2><p className="muted">使用现有 Wails bindings 发送测试告警。</p><button disabled={!ready} onClick={test}>发送测试告警</button></article><article><h2>Outbox</h2><span className="pill pending">占位</span><p className="muted">Outbox 队列尚未接入，当前不会持久化或自动发送。</p></article><article><h2>工作台状态</h2><p className="muted">连接：{status}</p><p className="muted">Intel：{intel ? `${intel.length} 字符` : '未输入'}</p></article></section>
    </>}</main>;
}
createRoot(document.getElementById('root')!).render(<App />);
