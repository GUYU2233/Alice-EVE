/// Notification settings synchronized with the server.
enum PushProvider { fcm, xiaomi, huawei, oppo, vivo }

extension PushProviderLabel on PushProvider {
  String get wireName => switch (this) { PushProvider.fcm => 'fcm', PushProvider.xiaomi => 'xiaomi', PushProvider.huawei => 'huawei', PushProvider.oppo => 'oppo', PushProvider.vivo => 'vivo' };
  String get label => switch (this) { PushProvider.fcm => 'FCM', PushProvider.xiaomi => '小米推送', PushProvider.huawei => '华为 Push Kit', PushProvider.oppo => 'OPPO PUSH', PushProvider.vivo => 'vivo 推送' };
  String get platform => 'android';

  static PushProvider? fromWireName(String? value) {
    final normalized = value?.trim().toLowerCase();
    for (final provider in PushProvider.values) {
      if (provider.wireName == normalized) return provider;
    }
    return null;
  }
}

/// Native SDK adapters implement this callback. Returning null means the
/// provider is not configured or the user has not granted permission yet.
typedef PushTokenRegistrationCallback = Future<String?> Function(PushProvider provider);

class ReminderPreferences {
  final bool enabled;
  final bool intelAlerts;
  final bool marketAlerts;
  final bool conversationUpdates;
  final String? quietHoursStart;
  final String? quietHoursEnd;
  final String? timezone;
  final bool sound;
  final bool vibration;

  const ReminderPreferences({
    this.enabled = true,
    this.intelAlerts = true,
    this.marketAlerts = true,
    this.conversationUpdates = true,
    this.quietHoursStart,
    this.quietHoursEnd,
    this.timezone,
    this.sound = true,
    this.vibration = true,
  });

  factory ReminderPreferences.fromJson(Map<String, dynamic> json) => ReminderPreferences(
        enabled: json['enabled'] as bool? ?? true,
        intelAlerts: json['intelAlerts'] as bool? ?? true,
        marketAlerts: json['marketAlerts'] as bool? ?? true,
        conversationUpdates: json['conversationUpdates'] as bool? ?? true,
        quietHoursStart: json['quietHoursStart'] as String?,
        quietHoursEnd: json['quietHoursEnd'] as String?,
        timezone: json['timezone'] as String?,
        sound: json['sound'] as bool? ?? true,
        vibration: json['vibration'] as bool? ?? true,
      );

  ReminderPreferences copyWith({
    bool? enabled,
    bool? intelAlerts,
    bool? marketAlerts,
    bool? conversationUpdates,
    String? quietHoursStart,
    String? quietHoursEnd,
    String? timezone,
    bool? sound,
    bool? vibration,
  }) => ReminderPreferences(
        enabled: enabled ?? this.enabled,
        intelAlerts: intelAlerts ?? this.intelAlerts,
        marketAlerts: marketAlerts ?? this.marketAlerts,
        conversationUpdates: conversationUpdates ?? this.conversationUpdates,
        quietHoursStart: quietHoursStart ?? this.quietHoursStart,
        quietHoursEnd: quietHoursEnd ?? this.quietHoursEnd,
        timezone: timezone ?? this.timezone,
        sound: sound ?? this.sound,
        vibration: vibration ?? this.vibration,
      );

  Map<String, dynamic> toJson() => {
        'enabled': enabled,
        'intelAlerts': intelAlerts,
        'marketAlerts': marketAlerts,
        'conversationUpdates': conversationUpdates,
        if (quietHoursStart != null) 'quietHoursStart': quietHoursStart,
        if (quietHoursEnd != null) 'quietHoursEnd': quietHoursEnd,
        if (timezone != null) 'timezone': timezone,
        'sound': sound,
        'vibration': vibration,
      };
}

class ReminderPreferencesDocument {
  final String deviceId;
  final int version;
  final String etag;
  final ReminderPreferences preferences;
  final DateTime updatedAt;

  const ReminderPreferencesDocument({required this.deviceId, required this.version, required this.etag, required this.preferences, required this.updatedAt});
  factory ReminderPreferencesDocument.fromJson(Map<String, dynamic> json) => ReminderPreferencesDocument(
        deviceId: json['deviceId'] as String,
        version: (json['version'] as num).toInt(),
        etag: json['etag'] as String,
        preferences: ReminderPreferences.fromJson(Map<String, dynamic>.from(json['preferences'] as Map)),
        updatedAt: DateTime.parse(json['updatedAt'] as String).toUtc(),
      );
}

class PushTokenRegistration {
  final String provider;
  final String platform;
  final String token;
  final String? appVersion;
  const PushTokenRegistration({required this.provider, required this.platform, required this.token, this.appVersion});
  Map<String, dynamic> toJson() => {'provider': provider, 'platform': platform, 'token': token, if (appVersion != null) 'appVersion': appVersion};
}

class RegisteredPushToken {
  final String id;
  final String deviceId;
  final String provider;
  final String platform;
  final DateTime createdAt;
  final DateTime updatedAt;
  const RegisteredPushToken({required this.id, required this.deviceId, required this.provider, required this.platform, required this.createdAt, required this.updatedAt});
  factory RegisteredPushToken.fromJson(Map<String, dynamic> json) => RegisteredPushToken(id: json['id'] as String, deviceId: json['deviceId'] as String, provider: json['provider'] as String, platform: json['platform'] as String, createdAt: DateTime.parse(json['createdAt'] as String).toUtc(), updatedAt: DateTime.parse(json['updatedAt'] as String).toUtc());
}
