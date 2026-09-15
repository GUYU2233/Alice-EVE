import 'package:flutter/services.dart';

import '../models/notifications.dart';

/// Provider ids are stable wire values shared with the Android registry.
/// This file intentionally reuses the single model enum used by the API layer.

class PushProviderCapability {
  final PushProvider provider;
  final bool supportsDataMessages;
  final bool supportsNotificationMessages;
  final bool requiresVendorDevice;
  final bool requiresVendorSdk;
  final Set<String> manifestPermissions;
  final Set<String> runtimePermissions;
  final String integrationNotes;

  const PushProviderCapability({
    required this.provider,
    required this.supportsDataMessages,
    required this.supportsNotificationMessages,
    required this.requiresVendorDevice,
    required this.requiresVendorSdk,
    required this.manifestPermissions,
    required this.runtimePermissions,
    required this.integrationNotes,
  });

  factory PushProviderCapability.fromMap(Map<Object?, Object?> map) {
    final provider = PushProviderLabel.fromWireName(map['provider'] as String?);
    if (provider == null) throw const FormatException('Unknown push provider');
    return PushProviderCapability(
      provider: provider,
      supportsDataMessages: map['supportsDataMessages'] as bool? ?? false,
      supportsNotificationMessages: map['supportsNotificationMessages'] as bool? ?? false,
      requiresVendorDevice: map['requiresVendorDevice'] as bool? ?? false,
      requiresVendorSdk: map['requiresVendorSdk'] as bool? ?? false,
      manifestPermissions: _stringSet(map['manifestPermissions']),
      runtimePermissions: _stringSet(map['runtimePermissions']),
      integrationNotes: map['integrationNotes'] as String? ?? '',
    );
  }

  static Set<String> _stringSet(Object? value) => value is List
      ? value.whereType<String>().toSet()
      : const <String>{};
}

class PushInitializationResult {
  final PushProvider provider;
  final bool ready;
  final String? reason;
  final String? token;

  const PushInitializationResult({required this.provider, required this.ready, this.reason, this.token});

  factory PushInitializationResult.fromMap(Map<Object?, Object?> map) {
    final provider = PushProviderLabel.fromWireName(map['provider'] as String?);
    if (provider == null) throw const FormatException('Unknown push provider');
    return PushInitializationResult(
      provider: provider,
      ready: map['ready'] as bool? ?? false,
      reason: map['reason'] as String?,
      token: map['token'] as String?,
    );
  }
}

/// Platform-neutral port for the Android provider registry.
///
/// The default implementation only queries declarative registry data and
/// returns "not configured" until an app build supplies an SDK adapter and
/// deployment configuration. It never accepts or stores provider secrets.
abstract interface class PushPlatform {
  Future<List<PushProviderCapability>> capabilities();
  Future<PushInitializationResult> initialize(PushProvider provider);
  Future<bool> requestNotificationPermission();
  Stream<String> get tokenChanges;
  Future<bool> getKeepAliveEnabled();
  Future<bool> setKeepAliveEnabled(bool enabled);
}

class MethodChannelPushPlatform implements PushPlatform {
  static const channel = MethodChannel('eve_assistant_mobile/push');
  const MethodChannelPushPlatform();

  @override
  Stream<String> get tokenChanges => const EventChannel('eve_assistant_mobile/push_tokens').receiveBroadcastStream().where((value) => value is String).cast<String>();

  @override
  Future<List<PushProviderCapability>> capabilities() async {
    final result = await channel.invokeMethod<List<Object?>>('getProviderCapabilities');
    return (result ?? const <Object?>[])
        .whereType<Map<Object?, Object?>>()
        .map(PushProviderCapability.fromMap)
        .toList(growable: false);
  }

  @override
  Future<PushInitializationResult> initialize(PushProvider provider) async {
    final result = await channel.invokeMethod<Object?>('initializeProvider', {'provider': provider.wireName});
    if (result is! Map<Object?, Object?>) {
      throw const FormatException('Invalid provider initialization response');
    }
    return PushInitializationResult.fromMap(result);
  }

  @override
  Future<bool> requestNotificationPermission() async =>
      await channel.invokeMethod<bool>('requestNotificationPermission') ?? false;

  @override
  Future<bool> getKeepAliveEnabled() async => await channel.invokeMethod<bool>('getKeepAliveEnabled') ?? false;

  @override
  Future<bool> setKeepAliveEnabled(bool enabled) async =>
      await channel.invokeMethod<bool>('setKeepAliveEnabled', {'enabled': enabled}) ?? enabled;
}
