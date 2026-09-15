import 'dart:async';

import 'package:flutter/material.dart';

import 'api/api_client.dart';
import 'api/conversation_client.dart';
import 'models/notifications.dart';
import 'notifications/push_platform.dart';
import 'models/protocol.dart';
import 'devices/device_management_page.dart';

void main() => runApp(const EveAssistantApp());

class EveAssistantApp extends StatefulWidget {
  const EveAssistantApp({super.key});

  @override
  State<EveAssistantApp> createState() => _EveAssistantAppState();
}

class _EveAssistantAppState extends State<EveAssistantApp> {
  ThemeMode themeMode = ThemeMode.system;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'EVE 助手',
      debugShowCheckedModeBanner: false,
      themeMode: themeMode,
      theme: _buildTheme(Brightness.light),
      darkTheme: _buildTheme(Brightness.dark),
      home: HomePage(
        themeMode: themeMode,
        onThemeModeChanged: (mode) => setState(() => themeMode = mode),
      ),
    );
  }
}

ThemeData _buildTheme(Brightness brightness) {
  final isDark = brightness == Brightness.dark;
  final scheme = ColorScheme.fromSeed(
    seedColor: const Color(0xFF7A8DFF),
    brightness: brightness,
    surface: isDark ? const Color(0xFF111318) : const Color(0xFFF9F9FD),
  );
  final outline = scheme.outlineVariant.withAlpha(isDark ? 150 : 110);
  return ThemeData(
    useMaterial3: true,
    brightness: brightness,
    colorScheme: scheme,
    scaffoldBackgroundColor: scheme.surface,
    visualDensity: VisualDensity.compact,
    appBarTheme: AppBarTheme(
      backgroundColor: scheme.surface,
      foregroundColor: scheme.onSurface,
      surfaceTintColor: Colors.transparent,
      scrolledUnderElevation: 0,
      elevation: 0,
    ),
    cardTheme: CardThemeData(
      margin: EdgeInsets.zero,
      elevation: 0,
      surfaceTintColor: Colors.transparent,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: BorderSide(color: outline),
      ),
    ),
    listTileTheme: const ListTileThemeData(dense: true, minVerticalPadding: 8),
    dividerTheme: DividerThemeData(color: outline, space: 1, thickness: 1),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: scheme.surfaceContainerHighest,
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(8),
        borderSide: BorderSide(color: outline),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(8),
        borderSide: BorderSide(color: outline),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(8),
        borderSide: BorderSide(color: scheme.primary, width: 2),
      ),
      contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 12),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        minimumSize: const Size(0, 40),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      ),
    ),
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: scheme.surface,
      surfaceTintColor: Colors.transparent,
      indicatorColor: scheme.secondaryContainer,
      height: 72,
      labelTextStyle: WidgetStatePropertyAll(
        TextStyle(fontSize: 12, color: scheme.onSurface),
      ),
      indicatorShape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
    ),
  );
}

class HomePage extends StatefulWidget {
  final ThemeMode themeMode;
  final ValueChanged<ThemeMode> onThemeModeChanged;

  const HomePage({required this.themeMode, required this.onThemeModeChanged, super.key});

  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
  final RelayApiClient api = RelayApiClient(baseUrl: 'https://relay.example.invalid');
  final PushPlatform pushPlatform = const MethodChannelPushPlatform();
  int tab = 0;
  bool restoringSession = true;

  StreamSubscription<String>? _fcmTokenSubscription;

  @override
  void initState() {
    super.initState();
    _fcmTokenSubscription = pushPlatform.tokenChanges.listen((token) async {
      if ((api.deviceToken == null || api.deviceId == null) || token.trim().isEmpty) return;
      try {
        await api.registerPushToken(PushTokenRegistration(provider: PushProvider.fcm.wireName, platform: PushProvider.fcm.platform, token: token));
      } catch (_) {
        // Registration retries on the next token event or explicit settings action.
      }
    });
    _restoreSession();
  }

  @override
  void dispose() {
    _fcmTokenSubscription?.cancel();
    super.dispose();
  }

  Future<void> _restoreSession() async {
    try {
      await api.restoreSession();
    } finally {
      if (mounted) setState(() => restoringSession = false);
    }
  }
  List<AlertEvent> alerts = const [];
  bool loading = false;
  String? loadError;
  DateTime? lastUpdated;

  bool get authorized => api.deviceToken != null && api.deviceId != null;

  Future<void> refresh() async {
    if (!authorized) return;
    setState(() {
      loading = true;
      loadError = null;
    });
    try {
      final fetched = await api.fetchAlerts(limit: 100);
      if (mounted) {
        setState(() {
          alerts = fetched;
          lastUpdated = DateTime.now();
        });
      }
    } catch (error) {
      if (mounted) setState(() => loadError = _friendlyError(error));
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  Future<void> sendCommand(String type, Map<String, dynamic> payload) async {
    if (!authorized) throw StateError('设备尚未授权');
    await api.sendReadOnlyRequest(type, payload);
  }

  void selectTab(int index) {
    setState(() => tab = index);
    if (index == 0 && authorized) refresh();
  }

  void openSettings() => setState(() => tab = 3);

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('EVE 助手', style: TextStyle(fontWeight: FontWeight.w700)),
        actions: [
          _AuthorizationChip(authorized: authorized),
          const SizedBox(width: 8),
          if (tab == 0)
            IconButton(
              tooltip: '刷新提醒',
              onPressed: loading || !authorized ? null : refresh,
              icon: loading
                  ? const SizedBox.square(dimension: 18, child: CircularProgressIndicator(strokeWidth: 2))
                  : const Icon(Icons.refresh_rounded),
            ),
          const SizedBox(width: 4),
        ],
      ),
      body: IndexedStack(
        index: tab,
        children: [
          AlertList(
            alerts: alerts,
            api: api,
            loading: loading,
            error: loadError,
            authorized: authorized,
            lastUpdated: lastUpdated,
            onRefresh: refresh,
            onOpenSettings: openSettings,
          ),
          DesktopControlPage(
            authorized: authorized,
            onCommand: sendCommand,
            onOpenSettings: openSettings,
          ),
          AgentPage(authorized: authorized, api: api, onCommand: sendCommand, onOpenSettings: openSettings),
          SettingsPage(
            api: api,
            onAuthorized: refresh,
            themeMode: widget.themeMode,
            onThemeModeChanged: widget.onThemeModeChanged,
            tokenProvider: (provider) async {
              final initialized = await pushPlatform.initialize(provider);
              return initialized.ready ? initialized.token : null;
            },
          ),
        ],
      ),
      bottomNavigationBar: NavigationBar(
        selectedIndex: tab,
        onDestinationSelected: selectTab,
        destinations: const [
          NavigationDestination(
            icon: Icon(Icons.notifications_none_rounded),
            selectedIcon: Icon(Icons.notifications_rounded),
            label: '提醒',
          ),
          NavigationDestination(
            icon: Icon(Icons.desktop_windows_outlined),
            selectedIcon: Icon(Icons.desktop_windows_rounded),
            label: '桌面端',
          ),
          NavigationDestination(
            icon: Icon(Icons.forum_outlined),
            selectedIcon: Icon(Icons.forum_rounded),
            label: 'Agent',
          ),
          NavigationDestination(
            icon: Icon(Icons.settings_outlined),
            selectedIcon: Icon(Icons.settings_rounded),
            label: '设置',
          ),
        ],
      ),
    );
  }
}

class _AuthorizationChip extends StatelessWidget {
  final bool authorized;
  const _AuthorizationChip({required this.authorized});

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Chip(
      avatar: Icon(
        authorized ? Icons.verified_user_outlined : Icons.lock_outline,
        size: 16,
        color: authorized ? scheme.primary : scheme.onSurfaceVariant,
      ),
      label: Text(authorized ? '已授权' : '未授权'),
      visualDensity: VisualDensity.compact,
      side: BorderSide(color: scheme.outlineVariant),
    );
  }
}

class AlertList extends StatefulWidget {
  final List<AlertEvent> alerts;
  final ApiClient api;
  final Future<void> Function() onRefresh;
  final VoidCallback onOpenSettings;
  final bool loading;
  final String? error;
  final bool authorized;
  final DateTime? lastUpdated;

  const AlertList({
    required this.alerts,
    required this.api,
    required this.onRefresh,
    required this.onOpenSettings,
    this.loading = false,
    this.error,
    this.authorized = false,
    this.lastUpdated,
    super.key,
  });

  @override
  State<AlertList> createState() => _AlertListState();
}

class _AlertListState extends State<AlertList> {
  String filter = 'all';

  List<AlertEvent> get visibleAlerts {
    if (filter == 'all') return widget.alerts;
    return widget.alerts.where((alert) => alert.severity.toLowerCase() == filter).toList();
  }

  @override
  Widget build(BuildContext context) {
    if (!widget.authorized) {
      return _UnpairedState(onOpenSettings: widget.onOpenSettings);
    }
    final visible = visibleAlerts;
    return RefreshIndicator(
      onRefresh: widget.onRefresh,
      child: CustomScrollView(
        physics: const AlwaysScrollableScrollPhysics(),
        slivers: [
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
            sliver: SliverToBoxAdapter(
              child: _PageHeader(
                title: '提醒中心',
                subtitle: widget.lastUpdated == null
                    ? '桌面端情报与市场通知'
                    : '更新于 ${_formatDateTime(widget.lastUpdated!)}',
              ),
            ),
          ),
          if (widget.loading && widget.alerts.isNotEmpty)
            const SliverToBoxAdapter(child: LinearProgressIndicator(minHeight: 2)),
          SliverToBoxAdapter(
            child: SingleChildScrollView(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 8),
              scrollDirection: Axis.horizontal,
              child: Row(
                children: [
                  _filterChip('all', '全部'),
                  const SizedBox(width: 8),
                  _filterChip('critical', '紧急'),
                  const SizedBox(width: 8),
                  _filterChip('high', '高'),
                  const SizedBox(width: 8),
                  _filterChip('medium', '中'),
                  const SizedBox(width: 8),
                  _filterChip('low', '低'),
                ],
              ),
            ),
          ),
          if (widget.loading && widget.alerts.isEmpty)
            const SliverFillRemaining(hasScrollBody: false, child: Center(child: CircularProgressIndicator()))
          else if (widget.error != null && widget.alerts.isEmpty)
            SliverFillRemaining(hasScrollBody: false, child: _ErrorState(message: widget.error!, onRetry: widget.onRefresh))
          else if (visible.isEmpty)
            const SliverFillRemaining(
              hasScrollBody: false,
              child: _EmptyState(
                icon: Icons.notifications_none_rounded,
                title: '暂无提醒',
                message: '新的情报会在桌面端同步后出现在这里。',
              ),
            )
          else
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 24),
              sliver: SliverList.builder(
                itemCount: visible.length,
                itemBuilder: (context, index) => _AlertTile(
                  alert: visible[index],
                  api: widget.api,
                  onRefresh: widget.onRefresh,
                ),
              ),
            ),
        ],
      ),
    );
  }

  Widget _filterChip(String value, String label) => FilterChip(
        label: Text(label),
        selected: filter == value,
        onSelected: (_) => setState(() => filter = value),
        visualDensity: VisualDensity.compact,
        showCheckmark: true,
      );
}

class _AlertTile extends StatefulWidget {
  final AlertEvent alert;
  final ApiClient api;
  final Future<void> Function() onRefresh;
  const _AlertTile({required this.alert, required this.api, required this.onRefresh});

  @override
  State<_AlertTile> createState() => _AlertTileState();
}

class _AlertTileState extends State<_AlertTile> {
  bool busy = false;

  Future<void> acknowledge() async {
    setState(() => busy = true);
    try {
      await widget.api.acknowledgeAlert(widget.alert.id);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('提醒已确认')));
        await widget.onRefresh();
      }
    } catch (error) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(_friendlyError(error))));
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final color = _severityColor(widget.alert.severity, scheme);
    return Card(
      margin: const EdgeInsets.only(top: 8),
      child: Column(
        children: [
          ListTile(
            dense: true,
            leading: Icon(_severityIcon(widget.alert.severity), color: color),
            title: Text(widget.alert.title, maxLines: 2, overflow: TextOverflow.ellipsis),
            subtitle: Padding(
              padding: const EdgeInsets.only(top: 4),
              child: Text(widget.alert.summary, maxLines: 3, overflow: TextOverflow.ellipsis),
            ),
            trailing: Text(_formatTime(widget.alert.createdAt), style: Theme.of(context).textTheme.labelSmall),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 8, 4),
            child: Row(
              children: [
                Expanded(
                  child: Wrap(
                    spacing: 4,
                    runSpacing: 4,
                    children: [
                      _Tag(label: _severityLabel(widget.alert.severity), color: color),
                      if (widget.alert.systemName != null) _Tag(label: widget.alert.systemName!),
                      if (widget.alert.source != null) _Tag(label: widget.alert.source!),
                      if (widget.alert.evidenceCount != null) _Tag(label: '${widget.alert.evidenceCount} 条证据'),
                    ],
                  ),
                ),
                TextButton.icon(
                  onPressed: busy ? null : acknowledge,
                  icon: busy
                      ? const SizedBox.square(dimension: 14, child: CircularProgressIndicator(strokeWidth: 2))
                      : const Icon(Icons.done_rounded, size: 17),
                  label: const Text('确认'),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _Tag extends StatelessWidget {
  final String label;
  final Color? color;
  const _Tag({required this.label, this.color});

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return InputChip(
      label: Text(label),
      labelStyle: TextStyle(color: color ?? scheme.onSurfaceVariant, fontSize: 11),
      visualDensity: VisualDensity.compact,
      padding: EdgeInsets.zero,
      side: BorderSide(color: color?.withAlpha(120) ?? scheme.outlineVariant),
      onPressed: null,
    );
  }
}

class DesktopControlPage extends StatefulWidget {
  final bool authorized;
  final Future<void> Function(String, Map<String, dynamic>) onCommand;
  final VoidCallback onOpenSettings;
  const DesktopControlPage({required this.authorized, required this.onCommand, required this.onOpenSettings, super.key});

  @override
  State<DesktopControlPage> createState() => _DesktopControlPageState();
}

class _DesktopControlPageState extends State<DesktopControlPage> {
  bool connectionEnabled = true;
  bool quiet = false;
  bool busy = false;

  Future<void> chooseAction(String action, String title, String subtitle) async {
    final confirmed = await showModalBottomSheet<bool>(
      context: context,
      showDragHandle: true,
      builder: (context) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(title, style: Theme.of(context).textTheme.titleMedium),
              const SizedBox(height: 8),
              Text(subtitle, style: Theme.of(context).textTheme.bodyMedium),
              const SizedBox(height: 16),
              FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('发送请求')),
              const SizedBox(height: 8),
              TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
            ],
          ),
        ),
      ),
    );
    if (confirmed != true || !mounted) return;
    setState(() => busy = true);
    try {
      await widget.onCommand('desktop.control', {'action': action});
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('已发送：$title')));
    } catch (error) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(_friendlyError(error))));
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (!widget.authorized) return _UnpairedState(onOpenSettings: widget.onOpenSettings);
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
      children: [
        const _PageHeader(title: '桌面端', subtitle: '只读控制 · 不会执行危险操作'),
        Card(
          child: ListTile(
            leading: const Icon(Icons.desktop_windows_rounded),
            title: const Text('Alice-EVE Desktop'),
            subtitle: const Text('最近心跳 · 2 分钟前'),
            trailing: Chip(label: const Text('在线'), visualDensity: VisualDensity.compact),
          ),
        ),
        const SizedBox(height: 16),
        const _PageHeader(title: '快捷操作', subtitle: '请求将通过已授权设备发送'),
        Card(
          child: Column(
            children: [
              _ActionTile(icon: Icons.sync_rounded, title: '同步情报', subtitle: '拉取最新提醒', onTap: busy ? null : () => chooseAction('sync_alerts', '同步情报', '从桌面端拉取最新提醒。')),
              const Divider(indent: 56),
              _ActionTile(icon: Icons.content_paste_rounded, title: '读取剪贴板', subtitle: '仅发送到移动端', onTap: busy ? null : () => chooseAction('read_clipboard', '读取剪贴板', '仅将当前剪贴板内容发送到本设备。')),
              const Divider(indent: 56),
              _ActionTile(icon: Icons.open_in_new_rounded, title: '打开主面板', subtitle: '唤起桌面窗口', onTap: busy ? null : () => chooseAction('open_dashboard', '打开主面板', '请求桌面端唤起主面板。')),
              const Divider(indent: 56),
              _ActionTile(icon: Icons.pause_circle_outline_rounded, title: '停止监听', subtitle: '暂停情报采集', destructive: true, onTap: busy ? null : () => chooseAction('pause_intel', '停止监听', '桌面端将暂停情报采集，仍可手动恢复。')),
            ],
          ),
        ),
        const SizedBox(height: 16),
        Card(
          child: Column(
            children: [
              SwitchListTile.adaptive(value: connectionEnabled, onChanged: (value) => setState(() => connectionEnabled = value), title: const Text('桌面端连接'), subtitle: const Text('允许桌面端推送提醒'), secondary: const Icon(Icons.link_rounded)),
              const Divider(indent: 56),
              SwitchListTile.adaptive(value: quiet, onChanged: (value) => setState(() => quiet = value), title: const Text('免打扰模式'), subtitle: const Text('仍接收高优先级告警'), secondary: const Icon(Icons.do_not_disturb_on_outlined)),
            ],
          ),
        ),
      ],
    );
  }
}

class _ActionTile extends StatelessWidget {
  final IconData icon;
  final String title;
  final String subtitle;
  final bool destructive;
  final VoidCallback? onTap;
  const _ActionTile({required this.icon, required this.title, required this.subtitle, this.destructive = false, this.onTap});

  @override
  Widget build(BuildContext context) => ListTile(
        leading: Icon(icon, color: destructive ? Theme.of(context).colorScheme.error : null),
        title: Text(title),
        subtitle: Text(subtitle),
        trailing: const Icon(Icons.chevron_right_rounded),
        onTap: onTap,
      );
}

class AgentPage extends StatefulWidget {
  final bool authorized;
  final RelayApiClient api;
  final Future<void> Function(String, Map<String, dynamic>) onCommand;
  final VoidCallback onOpenSettings;
  const AgentPage({required this.authorized, required this.api, required this.onCommand, required this.onOpenSettings, super.key});

  @override
  State<AgentPage> createState() => _AgentPageState();
}

class _AgentPageState extends State<AgentPage> {
  final input = TextEditingController();
  final messages = <_ChatMessage>[
    const _ChatMessage(false, '我是 EVE 助手。可以帮你整理情报、查询星系状态，或解释最近的提醒。'),
  ];
  ConversationClient? conversation;
  StreamSubscription<ConversationEvent>? eventSubscription;
  bool sending = false;

  @override
  void initState() {
    super.initState();
    _startRealtime();
  }

  Future<void> _startRealtime() async {
    // A conversation is created by the desktop pairing flow. Until an id is
    // available, retain the legacy command path and avoid opening a socket.
    final id = widget.api.accountId;
    if (!widget.authorized || id == null || id.isEmpty) return;
  }

  @override
  void dispose() {
    eventSubscription?.cancel();
    conversation?.close();
    input.dispose();
    super.dispose();
  }

  Future<void> send() async {
    final text = input.text.trim();
    if (text.isEmpty || sending || !widget.authorized) return;
    input.clear();
    setState(() {
      messages.add(_ChatMessage(true, text));
      sending = true;
    });
    try {
      await widget.onCommand('agent.chat', {'text': text});
      if (mounted) setState(() => messages.add(const _ChatMessage(false, '已发送给桌面 Agent，稍后会把结果推送回来。')));
    } catch (error) {
      if (mounted) setState(() => messages.add(_ChatMessage(false, _friendlyError(error))));
    } finally {
      if (mounted) setState(() => sending = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (!widget.authorized) return _UnpairedState(onOpenSettings: widget.onOpenSettings);
    return Column(
      children: [
        const Padding(
          padding: EdgeInsets.fromLTRB(16, 8, 16, 0),
          child: _PageHeader(title: 'Agent 会话', subtitle: '请求会转发到已授权的桌面端'),
        ),
        Expanded(
          child: ListView.builder(
            padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
            itemCount: messages.length + (sending ? 1 : 0),
            itemBuilder: (context, index) {
              if (sending && index == messages.length) {
                return const Align(alignment: Alignment.centerLeft, child: Padding(padding: EdgeInsets.all(8), child: CircularProgressIndicator(strokeWidth: 2)));
              }
              final message = messages[index];
              final scheme = Theme.of(context).colorScheme;
              return Align(
                alignment: message.fromUser ? Alignment.centerRight : Alignment.centerLeft,
                child: Container(
                  constraints: const BoxConstraints(maxWidth: 340),
                  margin: const EdgeInsets.only(bottom: 8),
                  padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
                  decoration: BoxDecoration(
                    color: message.fromUser ? scheme.primaryContainer : scheme.surfaceContainerHighest,
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: Text(message.text, style: TextStyle(color: message.fromUser ? scheme.onPrimaryContainer : scheme.onSurfaceVariant)),
                ),
              );
            },
          ),
        ),
        SafeArea(
          top: false,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(16, 8, 16, 12),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: [
                Expanded(child: TextField(controller: input, minLines: 1, maxLines: 4, textInputAction: TextInputAction.newline, decoration: const InputDecoration(hintText: '向 Agent 提问…'))),
                const SizedBox(width: 8),
                IconButton.filled(tooltip: '发送', onPressed: sending ? null : send, icon: const Icon(Icons.arrow_upward_rounded)),
              ],
            ),
          ),
        ),
      ],
    );
  }
}

class _ChatMessage {
  final bool fromUser;
  final String text;
  const _ChatMessage(this.fromUser, this.text);
}

class SettingsPage extends StatefulWidget {
  final RelayApiClient api;
  final Future<void> Function() onAuthorized;
  final ThemeMode themeMode;
  final ValueChanged<ThemeMode> onThemeModeChanged;
  /// Native push integrations provide this callback from the host app.
  final PushTokenRegistrationCallback? tokenProvider;
  const SettingsPage({required this.api, required this.onAuthorized, required this.themeMode, required this.onThemeModeChanged, this.tokenProvider, super.key});

  @override
  State<SettingsPage> createState() => _SettingsPageState();
}

class _SettingsPageState extends State<SettingsPage> {
  bool dense = true;
  bool keepAlive = false;
  PushProvider provider = PushProvider.fcm;
  bool providerConfigured = false;
  bool busy = false;
  String? tokenId;
  ReminderPreferences preferences = const ReminderPreferences();
  ReminderPreferencesDocument? preferencesDocument;

  bool get push => preferences.enabled;

  @override
  void initState() {
    super.initState();
    _loadNotificationState();
    _loadKeepAliveState();
  }

  Future<void> _loadKeepAliveState() async {
    try {
      final enabled = await const MethodChannelPushPlatform().getKeepAliveEnabled();
      if (mounted) setState(() => keepAlive = enabled);
    } catch (_) {}
  }

  Future<void> _toggleKeepAlive(bool enabled) async {
    setState(() => busy = true);
    try {
      final actual = await const MethodChannelPushPlatform().setKeepAliveEnabled(enabled);
      if (mounted) {
        setState(() => keepAlive = actual);
        _showMessage(actual ? '后台保活已开启，系统将显示常驻通知' : '后台保活已关闭');
      }
    } catch (error) {
      if (mounted) _showMessage('无法切换后台保活：${_friendlyError(error)}');
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> _loadNotificationState() async {
    if (widget.api.deviceToken == null) return;
    try {
      final loaded = await widget.api.fetchReminderPreferences();
      final tokens = await widget.api.fetchPushTokens();
      if (!mounted) return;
      setState(() {
        preferencesDocument = loaded;
        preferences = loaded.preferences;
        final current = tokens.where((item) => item.provider == provider.wireName).firstOrNull;
        tokenId = current?.id;
        providerConfigured = current != null;
      });
    } catch (_) {
      // The settings page remains usable when the relay is not reachable.
    }
  }

  Future<void> _togglePreference(String name, bool value) async {
    final document = preferencesDocument;
    if (document == null) {
      setState(() => preferences = switch (name) {
        'enabled' => preferences.copyWith(enabled: value),
        'intelAlerts' => preferences.copyWith(intelAlerts: value),
        'marketAlerts' => preferences.copyWith(marketAlerts: value),
        _ => preferences.copyWith(conversationUpdates: value),
      });
      return;
    }
    final next = switch (name) {
      'enabled' => preferences.copyWith(enabled: value),
      'intelAlerts' => preferences.copyWith(intelAlerts: value),
      'marketAlerts' => preferences.copyWith(marketAlerts: value),
      _ => preferences.copyWith(conversationUpdates: value),
    };
    setState(() { preferences = next; busy = true; });
    try {
      final updated = await widget.api.updateReminderPreferences(next, baseVersion: document.version, ifMatch: document.etag);
      if (mounted) setState(() => preferencesDocument = updated);
    } catch (error) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(_friendlyError(error))));
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> _registerProvider() async {
    final callback = widget.tokenProvider;
    if (callback == null) {
      _showMessage('未配置：请先接入 ${provider.label} 原生 SDK 并等待授权');
      return;
    }
    setState(() => busy = true);
    try {
      final token = await callback(provider);
      if (token == null || token.trim().isEmpty) {
        _showMessage('等待授权：${provider.label} 尚未返回 token');
        return;
      }
      final registered = await widget.api.registerPushToken(PushTokenRegistration(provider: provider.wireName, platform: provider.platform, token: token));
      if (mounted) setState(() { providerConfigured = true; tokenId = registered.id; });
      _showMessage('${provider.label} 已注册');
    } catch (error) {
      if (mounted) _showMessage(_friendlyError(error));
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> _revokePushProvider() async {
    final id = tokenId;
    if (id == null) { _showMessage('当前没有已注册的推送 token'); return; }
    setState(() => busy = true);
    try {
      await widget.api.revokePushToken(id);
      if (mounted) setState(() { providerConfigured = false; tokenId = null; });
      _showMessage('已撤销 ${provider.label}');
    } catch (error) {
      if (mounted) _showMessage(_friendlyError(error));
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  void _showMessage(String message) => ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(message)));

  Future<void> _showQuietHours() => _editQuietHours();

  TimeOfDay? _parseTime(String? value) {
    if (value == null) return null;
    final parts = value.split(':');
    if (parts.length != 2) return null;
    final hour = int.tryParse(parts[0]);
    final minute = int.tryParse(parts[1]);
    if (hour == null || minute == null || hour < 0 || hour > 23 || minute < 0 || minute > 59) return null;
    return TimeOfDay(hour: hour, minute: minute);
  }

  String _timeValue(TimeOfDay value) => '${value.hour.toString().padLeft(2, '0')}:${value.minute.toString().padLeft(2, '0')}';

  Future<void> _editQuietHours() async {
    final start = await showTimePicker(context: context, initialTime: _parseTime(preferences.quietHoursStart) ?? const TimeOfDay(hour: 22, minute: 0));
    if (start == null || !mounted) return;
    final end = await showTimePicker(context: context, initialTime: _parseTime(preferences.quietHoursEnd) ?? const TimeOfDay(hour: 7, minute: 0));
    if (end == null || !mounted) return;
    final next = preferences.copyWith(quietHoursStart: _timeValue(start), quietHoursEnd: _timeValue(end));
    final document = preferencesDocument;
    if (document == null) { setState(() => preferences = next); return; }
    setState(() => busy = true);
    try {
      final updated = await widget.api.updateReminderPreferences(next, baseVersion: document.version, ifMatch: document.etag);
      if (mounted) setState(() { preferences = updated.preferences; preferencesDocument = updated; });
    } catch (error) { if (mounted) _showMessage(_friendlyError(error)); }
    finally { if (mounted) setState(() => busy = false); }
  }

  @override
  Widget build(BuildContext context) {
    final authorized = widget.api.deviceToken != null && widget.api.deviceId != null;
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
      children: [
        const _PageHeader(title: '设置', subtitle: '设备、通知和显示偏好'),
        Card(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                ListTile(
                  contentPadding: EdgeInsets.zero,
                  leading: Icon(authorized ? Icons.verified_user_rounded : Icons.lock_outline_rounded),
                  title: Text(authorized ? '设备已授权' : '授权此设备'),
                  subtitle: Text(authorized ? '设备 ID：${widget.api.deviceId}' : '使用 EVE 账号身份完成安全授权'),
                  trailing: authorized ? OutlinedButton(onPressed: _revoke, child: const Text('撤销')) : null,
                ),
                if (!authorized) ...[
                  const Divider(),
                  const SizedBox(height: 8),
                  AccountAuthorizationPanel(api: widget.api, onAuthorized: widget.onAuthorized),
                ],
              ],
            ),
          ),
        ),
        const SizedBox(height: 16),
        if (authorized) ...[
           const SizedBox(height: 10),
           Card(
             child: ListTile(
               leading: const Icon(Icons.devices_other_rounded),
               title: const Text('账号设备'),
               subtitle: const Text('查看并撤销其他登录设备'),
               trailing: const Icon(Icons.chevron_right_rounded),
               onTap: () => Navigator.of(context).push(MaterialPageRoute(builder: (_) => DeviceManagementPage(api: widget.api))),
             ),
           ),
         ],
         const _PageHeader(title: '通知偏好', subtitle: '控制移动端提醒方式'),
        Card(
          child: Column(
            children: [
              SwitchListTile.adaptive(value: push, onChanged: (value) => _togglePreference('enabled', value), title: const Text('消息推送'), subtitle: Text(providerConfigured ? '${provider.label} · 已配置' : '${provider.label} · 等待系统授权'), secondary: const Icon(Icons.notifications_active_outlined)),
               SwitchListTile.adaptive(value: keepAlive, onChanged: busy ? null : _toggleKeepAlive, title: const Text('后台保活'), subtitle: const Text('开启后显示常驻通知，提升后台同步稳定性；不保证绕过系统省电策略'), secondary: const Icon(Icons.sync_rounded)),
              ListTile(leading: const Icon(Icons.hub_outlined), title: const Text('推送服务商'), subtitle: const Text('选择本机可用的消息通道'), trailing: DropdownButtonHideUnderline(child: DropdownButton<PushProvider>(value: provider, onChanged: (value) => setState(() { if (value != null) provider = value; providerConfigured = false; }), items: PushProvider.values.map((item) => DropdownMenuItem(value: item, child: Text(item.label))).toList()))),
              SwitchListTile.adaptive(value: preferences.intelAlerts, onChanged: push ? (value) => _togglePreference('intelAlerts', value) : null, title: const Text('情报提醒'), secondary: const Icon(Icons.radar_outlined)),
              SwitchListTile.adaptive(value: preferences.marketAlerts, onChanged: push ? (value) => _togglePreference('marketAlerts', value) : null, title: const Text('市场提醒'), secondary: const Icon(Icons.storefront_outlined)),
              SwitchListTile.adaptive(value: preferences.conversationUpdates, onChanged: push ? (value) => _togglePreference('conversationUpdates', value) : null, title: const Text('Agent 会话更新'), secondary: const Icon(Icons.forum_outlined)),
              ListTile(leading: const Icon(Icons.app_registration_outlined), title: const Text('注册当前推送通道'), subtitle: Text(providerConfigured ? '设备 token 已注册（仅保存 hash）' : '等待原生 SDK 返回 token'), trailing: providerConfigured ? OutlinedButton(onPressed: busy ? null : _revokePushProvider, child: const Text('撤销')) : OutlinedButton(onPressed: push && !busy ? _registerProvider : null, child: Text(busy ? '处理中…' : '注册'))),
              const Divider(indent: 56),
              ListTile(leading: const Icon(Icons.bedtime_outlined), title: const Text('静默时段'), subtitle: Text(preferences.quietHoursStart == null ? '未设置' : '${preferences.quietHoursStart} - ${preferences.quietHoursEnd}'), onTap: busy ? null : _showQuietHours),
               const Divider(indent: 56),
               SwitchListTile.adaptive(value: dense, onChanged: (value) => setState(() => dense = value), title: const Text('信息密度优先'), subtitle: const Text('使用紧凑间距显示更多上下文'), secondary: const Icon(Icons.density_small_rounded)),
            ],
          ),
        ),
        const SizedBox(height: 16),
        const _PageHeader(title: '显示', subtitle: '跟随系统或手动选择主题'),
        Card(
          child: ListTile(
            leading: const Icon(Icons.brightness_6_outlined),
            title: const Text('主题'),
            subtitle: Text(_themeLabel(widget.themeMode)),
            trailing: DropdownButtonHideUnderline(
              child: DropdownButton<ThemeMode>(
                value: widget.themeMode,
                onChanged: (mode) {
                  if (mode != null) widget.onThemeModeChanged(mode);
                },
                items: const [
                  DropdownMenuItem(value: ThemeMode.system, child: Text('系统')),
                  DropdownMenuItem(value: ThemeMode.light, child: Text('浅色')),
                  DropdownMenuItem(value: ThemeMode.dark, child: Text('深色')),
                ],
              ),
            ),
          ),
        ),
        const SizedBox(height: 16),
        const _PageHeader(title: '关于', subtitle: '版本与隐私信息'),
        Card(
          child: Column(
            children: const [
              ListTile(leading: Icon(Icons.shield_outlined), title: Text('隐私与安全'), subtitle: Text('不保存或上传 EVE SSO 凭据')),
              Divider(indent: 56),
              ListTile(leading: Icon(Icons.info_outline_rounded), title: Text('EVE 助手'), subtitle: Text('移动端 0.1.0')),
            ],
          ),
        ),
      ],
    );
  }

  Future<void> _revoke() async {
    final confirmed = await showModalBottomSheet<bool>(
      context: context,
      showDragHandle: true,
      builder: (context) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const ListTile(leading: Icon(Icons.logout_rounded), title: Text('撤销设备授权'), subtitle: Text('撤销后需要重新完成账号授权。')),
              const SizedBox(height: 8),
              FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('确认撤销')),
              const SizedBox(height: 8),
              TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
            ],
          ),
        ),
      ),
    );
    if (confirmed != true || !mounted) return;
    await widget.api.clearSession();
     if (!mounted) return;
     setState(() {});
    ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('设备授权已撤销')));
  }
}

class AccountAuthorizationPanel extends StatefulWidget {
  final ApiClient api;
  final Future<void> Function() onAuthorized;
  const AccountAuthorizationPanel({required this.api, required this.onAuthorized, super.key});

  @override
  State<AccountAuthorizationPanel> createState() => _AccountAuthorizationState();
}

class _AccountAuthorizationState extends State<AccountAuthorizationPanel> {
  final account = TextEditingController();
  final deviceName = TextEditingController(text: 'Alice-EVE mobile');
  bool busy = false;
  String? error;

  @override
  void dispose() {
    account.dispose();
    deviceName.dispose();
    super.dispose();
  }

  Future<void> authorize() async {
    final identity = account.text.trim();
    final name = deviceName.text.trim();
    if (identity.isEmpty) {
      setState(() => error = '请输入 EVE 账号或授权身份');
      return;
    }
    if (name.isEmpty) {
      setState(() => error = '请输入设备名称');
      return;
    }
    setState(() {
      busy = true;
      error = null;
    });
    try {
      await widget.api.authorizeDevice(account: identity, deviceName: name);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('设备授权成功，正在同步提醒')));
        await widget.onAuthorized();
      }
    } catch (error) {
      if (mounted) setState(() => this.error = _friendlyError(error));
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          TextField(controller: account, keyboardType: TextInputType.emailAddress, textInputAction: TextInputAction.next, decoration: const InputDecoration(labelText: 'EVE 账号 / 授权身份', hintText: 'name@example.com')),
          const SizedBox(height: 8),
          TextField(controller: deviceName, textInputAction: TextInputAction.done, onSubmitted: (_) => authorize(), decoration: const InputDecoration(labelText: '设备名称', hintText: '例如 Alice-EVE mobile')),
          if (error != null) ...[
            const SizedBox(height: 8),
            Text(error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
          ],
          const SizedBox(height: 12),
          FilledButton.icon(onPressed: busy ? null : authorize, icon: busy ? const SizedBox.square(dimension: 16, child: CircularProgressIndicator(strokeWidth: 2)) : const Icon(Icons.verified_user_rounded), label: Text(busy ? '授权中…' : '使用账号授权设备')),
          const SizedBox(height: 8),
          Text('将跳转至安全授权流程。移动端不会保存或上传 EVE SSO 密码。', style: Theme.of(context).textTheme.bodySmall),
        ],
      );
}

class _PageHeader extends StatelessWidget {
  final String title;
  final String subtitle;
  const _PageHeader({required this.title, required this.subtitle});

  @override
  Widget build(BuildContext context) => Padding(
        padding: const EdgeInsets.only(bottom: 8),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(title, style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 4),
            Text(subtitle, style: Theme.of(context).textTheme.bodySmall),
          ],
        ),
      );
}

class _UnpairedState extends StatelessWidget {
  final VoidCallback onOpenSettings;
  const _UnpairedState({required this.onOpenSettings});

  @override
  Widget build(BuildContext context) => Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(Icons.phonelink_lock_rounded, size: 44, color: Theme.of(context).colorScheme.primary),
              const SizedBox(height: 16),
              const Text('设备尚未授权', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700)),
              const SizedBox(height: 8),
              Text('前往设置使用 EVE 账号完成安全授权，解锁提醒、桌面端与 Agent 会话。', textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodyMedium),
              const SizedBox(height: 16),
              FilledButton.icon(onPressed: onOpenSettings, icon: const Icon(Icons.settings_outlined), label: const Text('前往设置')),
            ],
          ),
        ),
      );
}

class _EmptyState extends StatelessWidget {
  final IconData icon;
  final String title;
  final String message;
  const _EmptyState({required this.icon, required this.title, required this.message});

  @override
  Widget build(BuildContext context) => Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(icon, size: 42, color: Theme.of(context).colorScheme.outline),
              const SizedBox(height: 12),
              Text(title, style: const TextStyle(fontWeight: FontWeight.w700)),
              const SizedBox(height: 6),
              Text(message, textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodySmall),
            ],
          ),
        ),
      );
}

class _ErrorState extends StatelessWidget {
  final String message;
  final Future<void> Function() onRetry;
  const _ErrorState({required this.message, required this.onRetry});

  @override
  Widget build(BuildContext context) => Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(Icons.cloud_off_rounded, size: 42, color: Theme.of(context).colorScheme.error),
              const SizedBox(height: 12),
              const Text('暂时无法获取提醒', style: TextStyle(fontWeight: FontWeight.w700)),
              const SizedBox(height: 6),
              Text(message, textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodySmall),
              const SizedBox(height: 12),
              FilledButton.tonalIcon(onPressed: onRetry, icon: const Icon(Icons.refresh_rounded), label: const Text('重试')),
            ],
          ),
        ),
      );
}

IconData _severityIcon(String severity) => switch (severity.toLowerCase()) {
      'critical' => Icons.error_rounded,
      'high' => Icons.warning_rounded,
      'medium' => Icons.priority_high_rounded,
      'low' => Icons.info_outline_rounded,
      _ => Icons.notifications_rounded,
    };

Color _severityColor(String severity, ColorScheme scheme) => switch (severity.toLowerCase()) {
      'critical' => scheme.error,
      'high' => scheme.tertiary,
      'medium' => scheme.secondary,
      'low' => scheme.primary,
      _ => scheme.outline,
    };

String _severityLabel(String severity) => switch (severity.toLowerCase()) {
      'critical' => '紧急',
      'high' => '高',
      'medium' => '中',
      'low' => '低',
      _ => '普通',
    };

String _themeLabel(ThemeMode mode) => switch (mode) {
      ThemeMode.system => '跟随系统',
      ThemeMode.light => '浅色',
      ThemeMode.dark => '深色',
    };

String _formatTime(DateTime time) {
  final local = time.toLocal();
  return '${local.hour.toString().padLeft(2, '0')}:${local.minute.toString().padLeft(2, '0')}';
}

String _formatDateTime(DateTime time) {
  final local = time.toLocal();
  return '${local.month}/${local.day} ${_formatTime(local)}';
}

String _friendlyError(Object error) => error is RelayApiException
    ? error.message
    : error is UnsupportedError
        ? '当前操作暂不可用'
        : error is StateError
            ? error.message
            : '网络请求失败，请稍后重试';
