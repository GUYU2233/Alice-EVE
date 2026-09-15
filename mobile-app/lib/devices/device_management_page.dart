import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models/devices.dart';

class DeviceManagementPage extends StatefulWidget {
  final ApiClient api;

  const DeviceManagementPage({required this.api, super.key});

  @override
  State<DeviceManagementPage> createState() => _DeviceManagementPageState();
}

class _DeviceManagementPageState extends State<DeviceManagementPage> {
  List<AccountDevice> devices = const [];
  bool loading = true;
  bool busy = false;
  String? error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      loading = true;
      error = null;
    });
    try {
      final value = await widget.api.fetchDevices();
      if (mounted) setState(() => devices = value);
    } catch (e) {
      if (mounted) setState(() => error = _message(e));
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  Future<void> _revoke(AccountDevice device) async {
    final confirmed = await _confirm(
      title: device.current ? '撤销当前设备？' : '撤销此设备？',
      message: device.current ? '撤销后本机将退出账号，需要重新授权。' : '该设备将立即失去访问权限。',
      action: '撤销设备',
    );
    if (!confirmed || !mounted) return;
    setState(() => busy = true);
    try {
      await widget.api.revokeDevice(device.id);
      if (!mounted) return;
      if (device.current) {
        Navigator.of(context).pop();
        return;
      }
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('设备已撤销')));
      await _load();
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(_message(e))));
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> _revokeAll() async {
    final confirmed = await _confirm(
      title: '撤销全部设备？',
      message: '所有设备（包括当前设备）都会立即退出账号，此操作不可撤销。',
      action: '撤销全部',
    );
    if (!confirmed || !mounted) return;
    setState(() => busy = true);
    try {
      await widget.api.revokeAllDevices();
      if (mounted) Navigator.of(context).pop();
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(_message(e))));
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<bool> _confirm({required String title, required String message, required String action}) async {
    return await showModalBottomSheet<bool>(
          context: context,
          showDragHandle: true,
          builder: (context) => SafeArea(
            child: Padding(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
              child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                Text(title, style: Theme.of(context).textTheme.titleMedium),
                const SizedBox(height: 8),
                Text(message),
                const SizedBox(height: 16),
                FilledButton(onPressed: () => Navigator.pop(context, true), child: Text(action)),
                TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
              ]),
            ),
          ),
        ) ??
        false;
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Scaffold(
      appBar: AppBar(
        title: const Text('账号设备'),
        actions: [
          IconButton(tooltip: '刷新', onPressed: loading || busy ? null : _load, icon: const Icon(Icons.refresh_rounded)),
          if (devices.length > 1)
            IconButton(tooltip: '撤销全部', onPressed: busy ? null : _revokeAll, icon: Icon(Icons.logout_rounded, color: scheme.error)),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: _load,
        child: ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
          children: [
            Text('已登录设备', style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 4),
            Text('管理账号的登录会话，不使用配对码。', style: Theme.of(context).textTheme.bodySmall),
            const SizedBox(height: 16),
            if (loading)
              const Padding(padding: EdgeInsets.all(32), child: Center(child: CircularProgressIndicator()))
            else if (error != null)
              _ErrorPanel(message: error!, onRetry: _load)
            else if (devices.isEmpty)
              const Padding(padding: EdgeInsets.all(32), child: Center(child: Text('暂无已登录设备')))
            else
              ...devices.map((device) => _DeviceCard(device: device, onRevoke: busy ? null : () => _revoke(device))),
            if (!loading && error == null && devices.length > 1) ...[
              const SizedBox(height: 12),
              OutlinedButton.icon(
                onPressed: busy ? null : _revokeAll,
                icon: Icon(Icons.logout_rounded, color: scheme.error),
                label: const Text('撤销全部设备'),
                style: OutlinedButton.styleFrom(foregroundColor: scheme.error),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _DeviceCard extends StatelessWidget {
  final AccountDevice device;
  final VoidCallback? onRevoke;

  const _DeviceCard({required this.device, required this.onRevoke});

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final icon = switch (device.type.toLowerCase()) {
      'desktop' => Icons.desktop_windows_rounded,
      'tablet' => Icons.tablet_mac_rounded,
      _ => Icons.smartphone_rounded,
    };
    return Card(
      margin: const EdgeInsets.only(bottom: 10),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(4, 4, 8, 4),
        child: ListTile(
          leading: CircleAvatar(backgroundColor: scheme.secondaryContainer, child: Icon(icon, color: scheme.onSecondaryContainer)),
          title: Row(children: [Expanded(child: Text(device.name, maxLines: 1, overflow: TextOverflow.ellipsis)), if (device.current) const _CurrentBadge()]),
          subtitle: Padding(
            padding: const EdgeInsets.only(top: 4),
            child: Text('${_typeLabel(device.type)} · ${_lastActive(device.lastActiveAt)}'),
          ),
          trailing: device.current
              ? OutlinedButton(onPressed: onRevoke, child: const Text('撤销'))
              : IconButton(tooltip: '撤销设备', onPressed: onRevoke, icon: Icon(Icons.delete_outline_rounded, color: scheme.error)),
        ),
      ),
    );
  }
}

class _CurrentBadge extends StatelessWidget {
  const _CurrentBadge();

  @override
  Widget build(BuildContext context) => Padding(
        padding: const EdgeInsets.only(left: 8),
        child: Chip(
          label: const Text('当前设备'),
          visualDensity: VisualDensity.compact,
          padding: EdgeInsets.zero,
          labelStyle: TextStyle(fontSize: 11, color: Theme.of(context).colorScheme.onPrimaryContainer),
          backgroundColor: Theme.of(context).colorScheme.primaryContainer,
          side: BorderSide.none,
        ),
      );
}

class _ErrorPanel extends StatelessWidget {
  final String message;
  final VoidCallback onRetry;
  const _ErrorPanel({required this.message, required this.onRetry});

  @override
  Widget build(BuildContext context) => Card(
        child: Padding(
          padding: const EdgeInsets.all(20),
          child: Column(children: [
            const Icon(Icons.cloud_off_rounded),
            const SizedBox(height: 8),
            Text(message, textAlign: TextAlign.center),
            const SizedBox(height: 12),
            FilledButton.tonal(onPressed: onRetry, child: const Text('重试')),
          ]),
        ),
      );
}

String _typeLabel(String type) => switch (type.toLowerCase()) {
      'desktop' => '桌面端',
      'tablet' => '平板',
      'mobile' => '移动端',
      _ => '其他设备',
    };

String _lastActive(DateTime? time) {
  if (time == null) return '最后活动未知';
  final elapsed = DateTime.now().difference(time.toLocal());
  if (elapsed.inMinutes < 1) return '刚刚活动';
  if (elapsed.inHours < 1) return '${elapsed.inMinutes} 分钟前活动';
  if (elapsed.inDays < 1) return '${elapsed.inHours} 小时前活动';
  if (elapsed.inDays < 30) return '${elapsed.inDays} 天前活动';
  return '${time.month}/${time.day} 最后活动';
}

String _message(Object error) => error is RelayApiException ? error.message : '网络请求失败，请稍后重试';
