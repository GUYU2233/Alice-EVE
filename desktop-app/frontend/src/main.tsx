import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './style.css';

const OFFICIAL = 'https://relay.example.invalid';
const URL_KEY = 'alice.relay.url';
const MODE_KEY = 'alice.relay.mode';

type Finding = {
  kind: string; value: string; stance: string; source: string; summary: string;
  observedAt: string; expiresAt: string; confidence: number;
};
type OAuthStart = { authorizationUrl: string; state: string; codeVerifier: string; expiresIn?: number };
type OAuthCompletion = { accountId?: string; deviceId?: string; accessExpiresAt?: string; refreshExpiresAt?: string };
type Api = {
  RelayURL?: () => Promise<string> | string;
  SetRelayURL?: (url: string) => Promise<void> | void;
  BeginOAuthLogin?: (deviceName: string) => Promise<OAuthStart>;
  CompleteOAuthLogin?: (state: string, code: string, verifier: string) => Promise<OAuthCompletion>;
  AuthorizeDevice?: (accountId: string, deviceName: string) => Promise<{ accountId: string; deviceId: string }>;
  CheckRelayHealth?: () => Promise<void> | void;
  GeneratePairingCode?: () => Promise<string> | string;
  PairingState?: () => Promise<{ DeviceID?: string; RelayURL?: string; PairedAt?: string }>;
  OpenLocalAgentSession?: () => Promise<{ sessionId: string; expiresAt: string }>;
  CloseLocalAgentSession?: (sessionId: string) => Promise<void>;
  HandleLocalAgentRequest?: (request: string) => Promise<string>;
  ClearPairing?: () => Promise<void> | void;
  PublishTestAlert?: () => Promise<void> | void;
  ParseIntel?: (text: string) => Promise<Finding[]> | Finding[];
  FetchAlerts?: (cursor: number, limit: number) => Promise<{ items: AlertItem[]; Items?: AlertItem[]; cursor: number; Cursor?: number }>;
  FetchOutbox?: (cursor: number, limit: number) => Promise<{ items: OutboxItem[]; Items?: OutboxItem[]; cursor: number; Cursor?: number }>;
  FetchConversations?: () => Promise<ConversationItem[]>;
  FetchConversationMessages?: (conversationID: string, cursor: number, limit: number) => Promise<{ messages: ConversationMessage[]; nextCursor: number; hasMore: boolean }>;
  SendAgentMessage?: (conversationID: string, clientMessageID: string, operation: string, body: string) => Promise<{ message?: ConversationMessage; duplicate?: boolean }>;
  StartAgentRun?: (conversationID: string, messageID: string) => Promise<AgentRun>;
  GetAgentRun?: (conversationID: string, runID: string) => Promise<AgentRun>;
  ListAgentRunEvents?: (conversationID: string, runID: string, after: number, limit: number) => Promise<AgentRunEvent[]>;
  CancelAgentRun?: (conversationID: string, runID: string) => Promise<AgentRun>;
  StartRealtime?: (accountID: string, deviceID: string) => Promise<void>;
  StopRealtime?: () => Promise<void>;
  RealtimeStatus?: () => Promise<{ state: string; cursor: number; retryInMs?: number; lastError?: string }>;
  NextRealtimeEvent?: () => Promise<RealtimeEvent | boolean>;
  AckRealtimeEvent?: (event: RealtimeEvent, status: string) => Promise<void>;
  FetchRealtimeSnapshot?: () => Promise<ReconcileSnapshot | string>;
  SetSDEDirectory?: (path: string) => Promise<void>;
  SDEDirectory?: () => Promise<string> | string;
  ReindexSDE?: () => Promise<{ directory: string; indexedAt: string; files: number; entries: number; ready: boolean; version?: string; source?: string; checksum?: string; signature?: string; indexVersion?: number }>;
  SDEStatus?: () => Promise<{ directory: string; indexedAt: string; files: number; entries: number; ready: boolean; version?: string; source?: string; checksum?: string; signature?: string; indexVersion?: number }>;
  QuerySDE?: (query: string, limit: number) => Promise<SDEEntry[]>;
};
type SDEEntry = { id: string; name: string; kind?: string; source?: string };
type AlertItem = { id?: string; ID?: string; type?: string; Type?: string; title?: string; Title?: string; summary?: string; Summary?: string; severity?: string; Severity?: string; createdAt?: string; CreatedAt?: string; status?: string; Status?: string };
type OutboxItem = AlertItem & { attempts?: number; Attempts?: number };
type ConversationItem = { id?: string; ID?: string; agentKind?: string; AgentKind?: string; status?: string; Status?: string; targetDeviceId?: string; TargetDeviceID?: string };
type ConversationMessage = { id?: string; ID?: string; body?: string; Body?: string; status?: string; Status?: string; operation?: string; Operation?: string; senderKind?: string; SenderKind?: string; createdAt?: string; CreatedAt?: string };
type AgentRun = { id: string; conversationId: string; inputMessageId?: string; status: string; errorCode?: string; startedAt?: string; completedAt?: string };
type AgentRunEvent = { id: string; runId: string; sequence: number; type: string; payload?: unknown; createdAt?: string };
type RealtimeEvent = { id: string; type: string; cursor: number; payload?: unknown; createdAt?: string };
type ReconcileSnapshot = { snapshotCursor: number; conversations: Array<{ id: string; messages?: ConversationMessage[]; runs?: AgentRun[] }> };

type ConnectionState = 'initializing' | 'ready' | 'connecting' | 'connected' | 'error';
type Nav = 'intel' | 'alerts' | 'agent' | 'queue';
type OAuthCallback = { code?: string; state?: string; error?: string };
type RuntimeWithBrowser = { BrowserOpenURL?: (url: string) => void };

function readSavedUrl() {
  try { return localStorage.getItem(URL_KEY) || OFFICIAL; } catch { return OFFICIAL; }
}
function readRelayMode(): 'official' | 'custom' {
  try { return localStorage.getItem(MODE_KEY) === 'custom' ? 'custom' : 'official'; } catch { return 'official'; }
}
function formatTime(value?: string) {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}
function statusMeta(state: ConnectionState) {
  return ({
    initializing: ['初始化中', 'busy'], ready: ['未连接', 'idle'], connecting: ['正在检查', 'busy'],
    connected: ['已连接', 'ok'], error: ['连接失败', 'danger'],
  } as Record<ConnectionState, [string, string]>)[state];
}

function App() {
  const api = (window as any).go?.main?.App as Api | undefined;
  const [relayMode, setRelayMode] = useState<'official' | 'custom'>(readRelayMode);
  const [url, setUrl] = useState(readSavedUrl);
  const [draftMode, setDraftMode] = useState<'official' | 'custom'>(relayMode);
  const [draftUrl, setDraftUrl] = useState(url);
  const [connection, setConnection] = useState<ConnectionState>('initializing');
  const [nav, setNav] = useState<Nav>('intel');
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [intel, setIntel] = useState('');
  const [findings, setFindings] = useState<Finding[]>([]);
  const [lastFindings, setLastFindings] = useState<Finding[]>([]);
  const [parseState, setParseState] = useState<'idle' | 'parsing' | 'success' | 'empty' | 'error'>('idle');
  const [parseAt, setParseAt] = useState('');
  const [pairCode, setPairCode] = useState('');
  const [pairing, setPairing] = useState(false);
  const [pairedDevice, setPairedDevice] = useState('');
  const [toast, setToast] = useState('');
  const [error, setError] = useState('');
  const [filter, setFilter] = useState('all');
  const [sending, setSending] = useState(false);
  const [outboxCount, setOutboxCount] = useState(0);
  const [alerts, setAlerts] = useState<AlertItem[]>([]);
  const [outbox, setOutbox] = useState<OutboxItem[]>([]);
  const [conversations, setConversations] = useState<ConversationItem[]>([]);
  const [dataLoading, setDataLoading] = useState(false);
  const [selectedConversation, setSelectedConversation] = useState('');
  const [agentBody, setAgentBody] = useState('');
  const [localAgent, setLocalAgent] = useState<{ sessionId: string; expiresAt: string } | null>(null);
  const [realtime, setRealtime] = useState<{ state: string; cursor: number; retryInMs?: number; lastError?: string }>({ state: 'stopped', cursor: 0 });
  const [conversationMessages, setConversationMessages] = useState<ConversationMessage[]>([]);
  const [messageCursor, setMessageCursor] = useState(0);
  const [activeRun, setActiveRun] = useState<AgentRun | null>(null);
  const [runEvents, setRunEvents] = useState<AgentRunEvent[]>([]);
  const [runEventCursor, setRunEventCursor] = useState(0);
  const [accountId, setAccountId] = useState('');
  const [deviceName, setDeviceName] = useState('Alice-EVE Desktop');
  const [oauthPending, setOauthPending] = useState<{ state: string; codeVerifier: string } | null>(null);
  const [oauthBusy, setOauthBusy] = useState(false);
  const [oauthStatus, setOauthStatus] = useState('');
  const [sdePath, setSdePath] = useState('');
  const [sdeQuery, setSdeQuery] = useState('');
  const [sdeResults, setSdeResults] = useState<SDEEntry[]>([]);
  const [sdeStatus, setSdeStatus] = useState<{entries:number;files:number;ready:boolean;version?:string;source?:string;checksum?:string;signature?:string;indexVersion?:number}>({entries:0,files:0,ready:false});

  const announce = useCallback((message: string) => {
    setToast(message);
    window.setTimeout(() => setToast(current => current === message ? '' : current), 3600);
  }, []);

  const completeOAuthCallback = useCallback(async (callback: OAuthCallback) => {
    if (!callback.error && callback.code && callback.state && oauthPending && callback.state === oauthPending.state && api?.CompleteOAuthLogin) {
      setOauthBusy(true); setError(''); setOauthStatus('正在完成登录…');
      try {
        const result = await api.CompleteOAuthLogin(callback.state, callback.code, oauthPending.codeVerifier);
        setOauthPending(null);
        if (result?.accountId) setAccountId(result.accountId);
        if (result?.deviceId) setPairedDevice(result.deviceId);
        setOauthStatus('登录成功'); announce('OAuth 登录成功，桌面设备已授权');
        void loadOperationalData();
      } catch { setOauthStatus('登录失败'); setError('OAuth 登录回调处理失败，请重新开始登录。'); }
      finally { setOauthBusy(false); }
    } else if (callback.error) {
      setOauthStatus('登录已取消'); setOauthPending(null); setError(`OAuth 登录未完成：${callback.error}`);
    }
  }, [api, announce, oauthPending]);

  const parseOAuthCallback = useCallback((value: string | URL | undefined): OAuthCallback => {
    try {
      const parsed = value instanceof URL ? value : new URL(value || window.location.href);
      return { code: parsed.searchParams.get('code') || undefined, state: parsed.searchParams.get('state') || undefined, error: parsed.searchParams.get('error') || undefined };
    } catch { return {}; }
  }, []);

  useEffect(() => {
    const handleCallback = (event: Event) => {
      const detail = (event as CustomEvent<unknown>).detail;
      if (detail && typeof detail === 'object' && ('code' in detail || 'state' in detail || 'error' in detail)) {
        const callback = detail as { code?: unknown; state?: unknown; error?: unknown };
        void completeOAuthCallback({ code: typeof callback.code === 'string' ? callback.code : undefined, state: typeof callback.state === 'string' ? callback.state : undefined, error: typeof callback.error === 'string' ? callback.error : undefined });
        return;
      }
      const raw = typeof detail === 'string' ? detail : detail && typeof detail === 'object' && 'url' in detail ? String((detail as { url?: unknown }).url || '') : undefined;
      void completeOAuthCallback(parseOAuthCallback(raw));
    };
    window.addEventListener('oauth-callback', handleCallback);
    window.addEventListener('oauth_callback', handleCallback);
    window.addEventListener('deep-link', handleCallback);
    void completeOAuthCallback(parseOAuthCallback(window.location.href));
    return () => {
      window.removeEventListener('oauth-callback', handleCallback);
      window.removeEventListener('oauth_callback', handleCallback);
      window.removeEventListener('deep-link', handleCallback);
    };
  }, [completeOAuthCallback, parseOAuthCallback]);

  const beginOAuthLogin = async () => {
    if (!api?.BeginOAuthLogin) { setError('当前桌面运行时不支持 OAuth 登录。'); return; }
    setOauthBusy(true); setError(''); setOauthStatus('正在准备登录…');
    try {
      const start = await api.BeginOAuthLogin(deviceName.trim() || 'Alice-EVE Desktop');
      if (!start?.authorizationUrl || !start.state || !start.codeVerifier) throw new Error('OAuth 登录参数不完整');
      setOauthPending({ state: start.state, codeVerifier: start.codeVerifier }); setOauthStatus('等待浏览器完成登录…');
      const runtime = (window as Window & { runtime?: RuntimeWithBrowser }).runtime;
      if (runtime?.BrowserOpenURL) runtime.BrowserOpenURL(start.authorizationUrl);
      else window.open(start.authorizationUrl, '_blank', 'noopener,noreferrer');
    } catch { setOauthStatus('登录启动失败'); setError('无法启动 OAuth 登录，请检查服务端连接后重试。'); }
    finally { setOauthBusy(false); }
  };

  useEffect(() => {
    if (!api) { setConnection('error'); setError('未检测到桌面运行时，请在 Alice-EVE 桌面应用中使用。'); return; }
    let cancelled = false;
    (async () => {
      try {
        const restored = await api.RelayURL?.();
        const next = restored || readSavedUrl();
        await api.SetRelayURL?.(next);
        const state = await api.PairingState?.().catch(() => undefined);
        if (!cancelled) {
          setUrl(next); setDraftUrl(next); setPairedDevice(state?.DeviceID || ''); setConnection('ready');
          void loadOperationalData();
        }
      } catch { if (!cancelled) { setConnection('error'); setError('本地配置加载失败，请在设置中检查服务器地址。'); } }
    })();
    return () => { cancelled = true; };
  }, [api]);

  const loadConversation = useCallback(async (conversationID: string) => {
    if (!api?.FetchConversationMessages || !conversationID) return;
    try { const page = await api.FetchConversationMessages(conversationID, 0, 100); setConversationMessages(page.messages || []); setMessageCursor(page.nextCursor || 0); } catch { setError('会话时间线加载失败。'); }
  }, [api]);

  const loadOperationalData = useCallback(async () => {
    if (!api) return;
    setDataLoading(true);
    try {
      const results = await Promise.allSettled([
        api.FetchAlerts?.(0, 50), api.FetchOutbox?.(0, 50), api.FetchConversations?.(),
      ]);
      const [alertResult, outboxResult, conversationResult] = results;
      if (alertResult.status === 'fulfilled' && alertResult.value) setAlerts(alertResult.value.items || alertResult.value.Items || []);
      if (outboxResult.status === 'fulfilled' && outboxResult.value) { const items = outboxResult.value.items || outboxResult.value.Items || []; setOutbox(items); setOutboxCount(items.length); }
      if (conversationResult.status === 'fulfilled' && conversationResult.value && Array.isArray(conversationResult.value)) { setConversations(conversationResult.value); if (!selectedConversation && conversationResult.value[0]) { const first = String(conversationResult.value[0].id || conversationResult.value[0].ID || ''); setSelectedConversation(first); void loadConversation(first); } }
      if (conversationResult.status === 'rejected' && alertResult.status === 'fulfilled' && outboxResult.status === 'fulfilled') setError('Agent 会话暂不可用，告警与发送队列仍可正常使用。');
    } catch { setError('运营数据加载失败，请确认设备已授权并重试。'); }
    finally { setDataLoading(false); }
  }, [api, loadConversation, selectedConversation]);

  const startRealtime = useCallback(async () => {
    if (!api?.StartRealtime || !accountId || !pairedDevice) return;
    try { await api.StartRealtime(accountId, pairedDevice); setRealtime(await api.RealtimeStatus?.() || { state: 'connecting', cursor: 0 }); } catch { setRealtime({ state: 'error', cursor: 0, lastError: '实时连接启动失败' }); }
  }, [accountId, api, pairedDevice]);

  useEffect(() => {
    if (!api?.NextRealtimeEvent || !pairedDevice) return;
    let cancelled = false;
    let reconciling = false;
    const poll = async () => { if (cancelled) return; try { const result = await api.NextRealtimeEvent?.(); if (result && typeof result === 'object' && 'id' in result) { const event = result as RealtimeEvent; const payload = event.payload && typeof event.payload === 'object' ? event.payload as Record<string, unknown> : {}; const eventRunID = typeof payload.runId === 'string' ? payload.runId : ''; if (event.type === 'conversation.message' && selectedConversation) void loadConversation(selectedConversation); if (api.AckRealtimeEvent) void api.AckRealtimeEvent(event, 'processed'); if (event.type === 'agent.run.event' && activeRun && eventRunID === activeRun.id) setRunEvents(events => events.some(item => item.id === event.id) ? events : [...events, { id: event.id, runId: activeRun.id, sequence: typeof payload.sequence === 'number' ? payload.sequence : event.cursor, type: typeof payload.type === 'string' ? payload.type : event.type, payload: payload.payload ?? payload, createdAt: event.createdAt }]); if (event.type.startsWith('agent.run.') && event.type !== 'agent.run.event' && activeRun && eventRunID === activeRun.id) { const statusValue = typeof payload.status === 'string' ? payload.status : ''; if (statusValue) setActiveRun(run => run ? { ...run, status: statusValue, errorCode: typeof payload.errorCode === 'string' ? payload.errorCode : run.errorCode } : run); } } const status = await api.RealtimeStatus?.(); if (status) { setRealtime(status); if (status.state === 'error' && status.lastError?.includes('reconcile') && !reconciling && api.FetchRealtimeSnapshot) { reconciling = true; try { const raw = await api.FetchRealtimeSnapshot(); const snapshot = typeof raw === 'string' ? JSON.parse(raw) as ReconcileSnapshot : raw; if (snapshot?.conversations) { const current = snapshot.conversations.find(item => item.id === selectedConversation); if (current) { setConversationMessages(current.messages || []); setActiveRun(current.runs?.[0] || null); } } announce('实时状态已完成快照恢复'); } catch { setError('实时游标已过期，快照恢复失败，请手动刷新。'); } finally { reconciling = false; } } } } catch (_) {} window.setTimeout(poll, 700); };
    void poll(); return () => { cancelled = true; };
  }, [activeRun, api, loadConversation, pairedDevice, selectedConversation]);

  useEffect(() => { if (connection === 'connected' && pairedDevice && accountId) void startRealtime(); }, [accountId, connection, pairedDevice, startRealtime]);

  const selectConversation = (id: string) => { setSelectedConversation(id); setActiveRun(null); setRunEvents([]); setRunEventCursor(0); void loadConversation(id); };
  const sendConversation = async () => {
    if (!api?.SendAgentMessage || !selectedConversation || !agentBody.trim()) return;
    const body = agentBody.trim(); setAgentBody('');
    try { const response = await api.SendAgentMessage(selectedConversation, `desktop-${Date.now()}`, 'get_desktop_status', body); if (response.message) setConversationMessages(messages => [...messages, response.message!]); const messageID = response.message?.id || response.message?.ID; if (messageID && api.StartAgentRun) { const run = await api.StartAgentRun(selectedConversation, messageID); setActiveRun(run); } } catch { setError('Agent 请求发送失败。'); }
  };
  const cancelRun = async () => { if (!activeRun || !api?.CancelAgentRun) return; try { setActiveRun(await api.CancelAgentRun(activeRun.conversationId, activeRun.id)); announce('Agent run 已取消'); } catch { setError('取消 Agent run 失败。'); } };

  const checkConnection = async () => {
    if (!api?.CheckRelayHealth) return;
    setError(''); setConnection('connecting');
    try { await api.CheckRelayHealth(); setConnection('connected'); announce('服务端连接正常'); void loadOperationalData(); }
    catch { setConnection('error'); setError('无法连接服务端，请检查地址、网络或代理配置。'); }
  };
  const authorizeDevice = async () => {
    if (!api?.AuthorizeDevice || !accountId.trim()) { setError('请输入账号 ID 后再授权。'); return; }
    try { const result = await api.AuthorizeDevice(accountId.trim(), deviceName.trim() || 'Alice-EVE Desktop'); setPairedDevice(result.deviceId); announce('桌面设备授权成功'); }
    catch { setError('设备授权失败。请先在手机端或已登录会话中完成账号授权。'); }
  };
  const generatePairing = async () => {
    if (!api?.GeneratePairingCode) return;
    setPairing(true); setError('');
    try { setPairCode(String(await api.GeneratePairingCode())); announce('配对码已生成，有效期 5 分钟'); }
    catch { setError('配对码生成失败，请先检查服务端连接。'); }
    finally { setPairing(false); }
  };
  const copyPairCode = async () => {
    if (!pairCode) return;
    try { await navigator.clipboard.writeText(pairCode); announce('配对码已复制'); }
    catch { announce(`配对码：${pairCode}`); }
  };
  const parseIntel = async () => {
    if (!intel.trim()) { setParseState('idle'); setError('请先粘贴 Intel 文本。'); return; }
    if (!api?.ParseIntel) return;
    setError(''); setParseState('parsing');
    try {
      const result = (await api.ParseIntel(intel)) || [];
      setFindings(result); setLastFindings(result); setParseAt(new Date().toISOString());
      setParseState(result.length ? 'success' : 'empty');
      announce(result.length ? `解析完成，发现 ${result.length} 条实体` : '解析完成，未识别到支持的实体');
    } catch { setParseState('error'); setError('解析失败，已保留上次成功结果。请稍后重试。'); }
  };
  const ensureLocalAgent = async () => {
    if (localAgent || !api?.OpenLocalAgentSession) return localAgent;
    const session = await api.OpenLocalAgentSession();
    setLocalAgent(session);
    return session;
  };
  const sendTestAlert = async () => {
    if (!api?.PublishTestAlert || connection !== 'connected') return;
    if (!window.confirm('这是一次模拟告警，不会代表真实威胁。确认发送？')) return;
    setSending(true); setError('');
    try { await api.PublishTestAlert(); setOutboxCount(count => count + 1); announce('模拟告警已发送'); }
    catch { setError('测试告警发送失败，请检查连接与手机设备状态。'); }
    finally { setSending(false); }
  };
  const saveSettings = async () => {
    const next = (draftMode === 'official' ? OFFICIAL : draftUrl).trim().replace(/\/$/, '');
    if (!/^https?:\/\/[^\s]+$/i.test(next)) { setError('自定义服务器地址必须是有效的 http(s) URL。'); return; }
    try { await api?.SetRelayURL?.(next); localStorage.setItem(URL_KEY, next); localStorage.setItem(MODE_KEY, draftMode); setRelayMode(draftMode); setUrl(next); setConnection('ready'); setSettingsOpen(false); announce('Alice 服务器配置已保存，请检查连接'); }
    catch { setError('服务器地址保存失败。'); }
  };
  const resetSettings = () => { setDraftMode('official'); setDraftUrl(OFFICIAL); announce('已选择 Alice 官方服务器，点击保存生效'); };
  const configureSDE = async () => {
    if (!api?.SetSDEDirectory || !sdePath.trim()) { setError('请输入本机 SDE 数据目录路径。'); return; }
    try { await api.SetSDEDirectory(sdePath.trim()); const status = await api.ReindexSDE?.(); if (status) setSdeStatus(status); announce('SDE 索引已更新'); }
    catch { setError('SDE 目录不可用或包含无法读取的文件。'); }
  };
  const searchSDE = async () => { if (!api?.QuerySDE || !sdeQuery.trim()) return; try { setSdeResults((await api.QuerySDE(sdeQuery, 20)) || []); } catch { setError('SDE 查询失败。'); } };
  const clearPairing = async () => {
    if (!window.confirm('确定清除本机授权状态？之后需要重新完成账号设备授权。')) return;
    try { await api?.ClearPairing?.(); setPairedDevice(''); setPairCode(''); announce('设备授权状态已清除'); }
    catch { setError('清除设备授权状态失败。'); }
  };

  const visibleFindings = useMemo(() => filter === 'all' ? findings : findings.filter(item => item.kind.toLowerCase().includes(filter) || item.stance.toLowerCase().includes(filter)), [filter, findings]);
  const [statusLabel, statusTone] = statusMeta(connection);
  const displayFindings = parseState === 'error' && !findings.length ? lastFindings : visibleFindings;

  return <div className="shell">
    <nav className="rail" aria-label="主导航">
      <div className="brand-mark" aria-label="Alice-EVE">A<span>·</span>E</div>
      <div className="rail-links">
        {([['intel', '⌁', '情报'], ['alerts', '◉', '告警'], ['agent', '□', 'Agent'], ['queue', '↑', '队列']] as [Nav, string, string][]).map(([key, icon, label]) => <button key={key} className={`rail-item ${nav === key ? 'active' : ''}`} onClick={() => setNav(key)} aria-current={nav === key ? 'page' : undefined}><span>{icon}</span><small>{label}</small>{key === 'queue' && outboxCount > 0 && <b className="nav-badge">{outboxCount}</b>}</button>)}
      </div>
      <button className={`rail-item rail-settings ${settingsOpen ? 'active' : ''}`} onClick={() => setSettingsOpen(true)} aria-label="打开设置"><span>⚙</span><small>设置</small></button>
    </nav>

    <div className="main-area">
      <header className="topbar">
        <div><div className="breadcrumb">ALICE-EVE / COMMAND DESK</div><h1>{nav === 'intel' ? '情报工作台' : nav === 'alerts' ? '告警中心' : nav === 'agent' ? 'Agent 会话' : '发送队列'} <em>v0.0.1-alpha</em></h1></div>
        <div className="top-actions"><div className="top-server"><span className={`dot ${statusTone}`} />{statusLabel}</div><button className="icon-button" onClick={checkConnection} title="检查连接">↻</button></div>
      </header>
      {!api && <div className="runtime-banner" role="alert">⚠ 未检测到桌面运行时。请通过 Alice-EVE 桌面应用打开此页面。<button onClick={() => window.location.reload()}>重新加载</button></div>}
      {error && <div className="global-error" role="alert"><span>!</span>{error}<button onClick={() => setError('')} aria-label="关闭错误">×</button></div>}

      {nav === 'intel' && <div className="workspace">
        <main className="content-column">
          <section className="m3-card composer-card">
            <div className="card-heading"><div><span className="section-kicker">CAPTURE</span><h2>Intel 原始输入</h2></div><span className="shortcut">Ctrl / ⌘ + Enter</span></div>
            <textarea autoFocus value={intel} onChange={event => setIntel(event.target.value)} onKeyDown={event => { if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') parseIntel(); }} placeholder="粘贴 EVE 频道文本…\n支持识别星系、舰船和立场等实体。" aria-label="Intel 原始文本" />
            <div className="composer-footer"><div className="helper"><span>{intel.length.toLocaleString()} 字符</span><span>·</span><span>本地规则解析</span><span className="privacy">▣ 隐私模式</span></div><div className="button-row"><button className="text-button" onClick={() => { setIntel(''); setFindings([]); setParseState('idle'); setError(''); }}>清空</button><button className="primary-button" disabled={!intel.trim() || parseState === 'parsing' || connection === 'initializing'} onClick={parseIntel}>{parseState === 'parsing' ? <><span className="spinner" />解析中</> : <>✦ 立即解析</>}</button></div></div>
          </section>

          <section className="results-section">
            <div className="section-heading"><div><span className="section-kicker">ANALYSIS</span><h2>解析发现 <span className="result-count">{findings.length}</span></h2>{parseAt && <p>本次解析于 {formatTime(parseAt)} 完成 · {intel.length} 字符</p>}</div><div className="filter-chips">{[['all', '全部'], ['system', '星系'], ['ship', '舰船'], ['hostile', '威胁']].map(([key, label]) => <button key={key} className={filter === key ? 'selected' : ''} onClick={() => setFilter(key)}>{label}</button>)}</div></div>
            {parseState === 'parsing' ? <div className="empty-state loading-state"><span className="spinner large" /><strong>正在解析输入</strong><p>提取实体、立场与置信度…</p></div> : !displayFindings.length ? <div className="empty-state"><div className="empty-icon">⌁</div><strong>{parseState === 'empty' ? '未识别到支持的实体' : '等待输入 Intel 文本'}</strong><p>{parseState === 'empty' ? '这不代表没有风险，请尝试粘贴包含星系或舰船名称的频道内容。' : '解析结果会在这里按置信度和威胁等级排列。'}</p></div> : <div className="table-wrap"><table><thead><tr><th>实体</th><th>摘要</th><th>立场</th><th>置信度</th><th>时间</th></tr></thead><tbody>{displayFindings.map((finding, index) => <tr key={`${finding.kind}-${finding.value}-${index}`}><td><strong>{finding.value}</strong><small>{finding.kind}</small></td><td className="summary">{finding.summary || '—'}<small>{finding.source || '本地解析'}</small></td><td><span className={`stance ${finding.stance.toLowerCase()}`}>{finding.stance || '未知'}</span></td><td><div className="confidence"><span className="confidence-bar"><i style={{ width: `${Math.max(0, Math.min(100, finding.confidence * 100))}%` }} /></span><code>{Math.round(finding.confidence * 100)}%</code></div></td><td><code>{formatTime(finding.observedAt)}</code></td></tr>)}</tbody></table></div>}
          </section>
        </main>
        <aside className="operations-column">
          <section className="m3-card connection-card"><div className="card-heading"><div><span className="section-kicker">SERVER</span><h2>服务端连接</h2></div><span className={`status-chip ${statusTone}`}><span className="dot" />{statusLabel}</span></div><p className="field-help server-mode-summary">{relayMode === 'official' ? 'Alice 官方服务器' : '自定义服务器'}</p><button className="tonal-button full" disabled={connection === 'connecting' || !api} onClick={checkConnection}>{connection === 'connecting' ? <><span className="spinner" />检查中…</> : '检查连接'}</button></section>
          <section className="m3-card pairing-card"><div className="card-heading"><div><span className="section-kicker">ACCOUNT DEVICE</span><h2>设备授权</h2></div><span className={`device-indicator ${pairedDevice ? 'linked' : ''}`}>{pairedDevice ? '已授权' : '未授权'}</span></div>{!pairedDevice && <><input value={deviceName} onChange={event => setDeviceName(event.target.value)} placeholder="设备名称" /><button className="primary-button full" disabled={oauthBusy || !api?.BeginOAuthLogin} onClick={beginOAuthLogin}>{oauthBusy ? <><span className="spinner" />处理中…</> : '使用 EVE SSO 登录'}</button>{oauthStatus && <small className="field-help">{oauthStatus}{oauthPending ? ' 完成浏览器登录后将自动返回。' : ''}</small>}<div className="field-help">也可使用已有账号 ID 进行设备授权</div><input value={accountId} onChange={event => setAccountId(event.target.value)} placeholder="账号 ID" /><button className="tonal-button full" disabled={connection !== 'connected' || !api?.AuthorizeDevice} onClick={authorizeDevice}>使用账号授权桌面设备</button></>}{pairedDevice && <small className="linked-device">设备 ID · {pairedDevice.slice(0, 18)}…</small>}</section>
          <section className="m3-card activity-card"><div className="card-heading"><div><span className="section-kicker">OPERATIONS</span><h2>操作面板</h2></div></div><button className="action-list-item" disabled={connection !== 'connected' || sending} onClick={sendTestAlert}><span className="action-icon warning">♢</span><span><strong>{sending ? '发送中…' : '发送模拟告警'}</strong><small>验证手机通知链路</small></span><span>›</span></button><div className="queue-summary"><span><b>{outboxCount}</b> 条待处理</span><span className="muted">Outbox {outboxCount ? '有新消息' : '为空'}</span></div></section>
          <section className="m3-card status-card"><div className="card-heading"><div><span className="section-kicker">WORKBENCH</span><h2>工作台状态</h2></div></div><div className="status-line"><span>连接</span><b className={statusTone}>{statusLabel}</b></div><div className="status-line"><span>配对设备</span><b>{pairedDevice ? '已绑定' : '未绑定'}</b></div><div className="status-line"><span>最近解析</span><b>{parseAt ? formatTime(parseAt) : '暂无'}</b></div></section>
        </aside>
      </div>}
      {nav === 'alerts' && <div className="data-view"><div className="data-view-heading"><div><span className="section-kicker">ALERTS</span><h2>告警中心</h2><p>来自已授权设备的真实服务端消息。</p></div><button className="tonal-button" onClick={loadOperationalData} disabled={dataLoading}>{dataLoading ? '刷新中…' : '刷新'}</button></div>{!alerts.length ? <div className="empty-state"><strong>暂无告警</strong><p>服务端没有返回新的告警消息。</p></div> : <div className="data-list">{alerts.map((item, index) => <div className="data-row" key={`${item.id || item.ID}-${index}`}><span className="action-icon warning">◉</span><div><strong>{item.title || item.Title || item.type || item.Type || '告警消息'}</strong><small>{item.summary || item.Summary || '收到服务端告警'} · {item.severity || item.Severity || 'info'}</small></div><code>{formatTime(item.createdAt || item.CreatedAt)}</code></div>)}</div>}</div>}
      {nav === 'queue' && <div className="data-view"><div className="data-view-heading"><div><span className="section-kicker">OUTBOX</span><h2>发送队列</h2><p>当前设备尚未确认的服务端消息。</p></div><button className="tonal-button" onClick={loadOperationalData} disabled={dataLoading}>{dataLoading ? '刷新中…' : '刷新'}</button></div>{!outbox.length ? <div className="empty-state"><strong>队列为空</strong><p>暂无待处理消息。</p></div> : <div className="data-list">{outbox.map((item, index) => <div className="data-row" key={`${item.id || item.ID}-${index}`}><span className="action-icon">↑</span><div><strong>{item.type || item.Type || '消息'}</strong><small>{item.status || item.Status || 'pending'} · {item.attempts || item.Attempts || 0} 次尝试</small></div><code>{formatTime(item.createdAt || item.CreatedAt)}</code></div>)}</div>}</div>}
      {nav === 'agent' && <div className="data-view"><div className="data-view-heading"><div><span className="section-kicker">AGENT</span><h2>Agent 会话</h2><p>与已授权桌面 Agent 建立只读会话。</p></div><button className="tonal-button" onClick={loadOperationalData} disabled={dataLoading}>{dataLoading ? '刷新中…' : '刷新'}</button></div>{!conversations.length ? <div className="empty-state"><strong>暂无可用会话</strong><p>服务端尚未返回账号范围内的 Agent 会话。</p></div> : <div className="agent-layout"><div className="data-list">{conversations.map((item, index) => { const id = String(item.id || item.ID || ''); return <button className={`conversation-row ${selectedConversation === id ? 'selected' : ''}`} key={`${id}-${index}`} onClick={() => selectConversation(id)}><strong>{item.agentKind || item.AgentKind || 'EVE Agent'}</strong><small>{item.status || item.Status || 'active'} · {item.targetDeviceId || item.TargetDeviceID || '设备未指定'}</small></button>; })}</div><div className="conversation-compose"><div className="conversation-timeline">{conversationMessages.map((message, index) => <article className="timeline-message" key={message.id || message.ID || index}><header><strong>{message.senderKind || message.SenderKind || 'message'}</strong><time>{formatTime(message.createdAt || message.CreatedAt)}</time></header><p>{message.body || message.Body || '—'}</p><small>{message.status || message.Status || 'persisted'}{message.operation || message.Operation ? ` · ${message.operation || message.Operation}` : ''}</small></article>)}</div><label>向会话发送只读请求<textarea value={agentBody} onChange={event => setAgentBody(event.target.value)} placeholder="例如：查询桌面当前状态" /></label><div className="run-controls"><button className="primary-button" disabled={!agentBody.trim() || !api?.SendAgentMessage} onClick={sendConversation}>发送并运行</button>{activeRun && <><span className="run-status">Run {activeRun.status}</span>{['queued','running'].includes(activeRun.status) && <button className="danger-button" onClick={cancelRun}>取消 Run</button>}</>}</div>{runEvents.length > 0 && <div className="run-events"><strong>运行事件</strong>{runEvents.map(event => <div key={event.id}><code>#{event.sequence}</code> {event.type} <small>{formatTime(event.createdAt)}</small></div>)}</div>}<button className="text-button" disabled={!agentBody.trim() || !api?.HandleLocalAgentRequest} onClick={async () => { try { const session = await ensureLocalAgent();
                if (!session || !api?.HandleLocalAgentRequest) throw new Error('本地 Agent 不可用');
                const response = await api.HandleLocalAgentRequest(JSON.stringify({ v: 1, id: `local-${Date.now()}`, sessionId: session.sessionId, operation: 'get_desktop_status', body: agentBody }));
                const parsed = JSON.parse(response);
                if (!parsed.ok) throw new Error(parsed.error?.message || '本地 Agent 拒绝请求');
                setAgentBody(''); announce('本地 Agent 已处理请求'); } catch { setError('Agent 请求发送失败，请检查会话权限。'); } }}>发送只读请求</button></div></div>}</div>}
      {nav !== 'intel' && nav !== 'alerts' && nav !== 'queue' && nav !== 'agent' && <div className="placeholder-view"><h2>工作台</h2></div>}
    </div>

    {settingsOpen && <div className="drawer-backdrop" onClick={() => setSettingsOpen(false)}><aside className="settings-drawer" onClick={event => event.stopPropagation()}><div className="drawer-heading"><div><span className="section-kicker">PREFERENCES</span><h2>工作台设置</h2></div><button className="icon-button" onClick={() => setSettingsOpen(false)} aria-label="关闭设置">×</button></div><section className="alice-server-settings"><h3>Alice 服务器设置</h3><div className="server-choice"><button type="button" className={draftMode === 'official' ? 'selected' : ''} onClick={() => setDraftMode('official')}><strong>Alice 官方服务器</strong><small>推荐，使用官方服务与最新能力</small></button><button type="button" className={draftMode === 'custom' ? 'selected' : ''} onClick={() => setDraftMode('custom')}><strong>自定义服务器</strong><small>连接你的私有或本地部署</small></button></div>{draftMode === 'custom' && <label>自定义服务器地址<input value={draftUrl} onChange={event => setDraftUrl(event.target.value)} placeholder="https://server.example" /></label>}<p className="field-help">凭据与设备 Token 不会显示在此处。</p></section><div className="drawer-actions"><button className="tonal-button" onClick={resetSettings}>恢复默认</button><button className="primary-button" onClick={saveSettings}>保存配置</button></div><div className="drawer-divider" /><h3>本机 SDE 数据</h3><label>数据目录<input value={sdePath} onChange={event => setSdePath(event.target.value)} placeholder="D:\\EVE\\sde" /></label><button className="tonal-button full" onClick={configureSDE}>选择并建立索引</button><p className="field-help">仅读取所选目录内的 JSON/JSONL/CSV；不会执行文件或访问任意 URL。{sdeStatus.ready ? ` 已索引 ${sdeStatus.entries} 条。` : ''}</p>{sdeStatus.ready && <div className="sde-metadata"><small>索引版本 {sdeStatus.indexVersion || 1} · {sdeStatus.version || '未声明版本'}</small><small>{sdeStatus.source || '本地目录'} · SHA-256 {sdeStatus.checksum ? sdeStatus.checksum.slice(0, 12) + '…' : '—'}</small>{sdeStatus.signature && <small>签名：已提供</small>}</div>}<label>查询 SDE<input value={sdeQuery} onChange={event => setSdeQuery(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') searchSDE(); }} placeholder="输入舰船、星系或物品名" /></label>{sdeResults.length > 0 && <div className="sde-results">{sdeResults.map((item, index) => <div key={`${item.id}-${index}`}><strong>{item.name}</strong><small>{item.id || '—'} · {item.source || '本地索引'}</small></div>)}</div>}<h3>本机数据</h3><button className="danger-button full" onClick={clearPairing}>清除设备授权</button><p className="field-help">清除后需要重新完成账号设备授权。</p></aside></div>}
    {toast && <div className="snackbar" role="status">✓ {toast}</div>}
  </div>;
}

createRoot(document.getElementById('root')!).render(<App />);
