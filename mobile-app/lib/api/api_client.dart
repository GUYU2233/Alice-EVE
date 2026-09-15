import 'dart:async';
import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:http/http.dart' as http;

import '../models/protocol.dart';
import '../models/notifications.dart';
import '../models/devices.dart';

abstract interface class ApiClient {
  /// Authorizes this mobile installation for an account identity.
  /// The identity assertion is exchanged for device-scoped credentials; EVE
  /// SSO passwords are never sent by this client.
  Future<void> authorizeDevice({required String account, required String deviceName});
  Future<void> restoreSession();
  Future<void> clearSession();
  Future<void> login(String account, String password);
  @Deprecated('Use authorizeDevice; pairing codes are no longer exposed in UI')
  Future<String> createPairingCode();
  @Deprecated('Use authorizeDevice; pairing codes are no longer exposed in UI')
  Future<void> confirmPairing(String code);
  Future<List<AlertEvent>> fetchAlerts({String? cursor, int? limit});
  Stream<AlertEvent> watchAlerts();
  Future<AcknowledgeResult> acknowledgeAlert(String id);
  Future<ReminderPreferencesDocument> fetchReminderPreferences();
  Future<ReminderPreferencesDocument> updateReminderPreferences(ReminderPreferences preferences, {required int baseVersion, required String ifMatch});
  Future<List<RegisteredPushToken>> fetchPushTokens();
  Future<RegisteredPushToken> registerPushToken(PushTokenRegistration registration);
  Future<void> revokePushToken(String tokenId);
  Future<List<AccountDevice>> fetchDevices();
  Future<void> revokeDevice(String deviceId);
  Future<void> revokeAllDevices();
  Future<void> sendReadOnlyRequest(String type, Map<String, dynamic> payload);
}

class RelayApiException implements Exception {
  final int statusCode;
  final String message;
  final Map<String, dynamic>? details;

  const RelayApiException(this.statusCode, this.message, {this.details});

  @override
  String toString() => 'RelayApiException($statusCode): $message';
}

/// HTTPS client for the mobile-facing server API.
///
/// The server issues a device-scoped token during pairing. Both the token and
/// device id are sent on subsequent requests; the latter prevents a token
/// accidentally being used for another device when a server supports multiple
/// devices per account.
class RelayApiClient implements ApiClient {
  final String baseUrl;
  final http.Client client;
  final FlutterSecureStorage secureStorage;
  String? deviceToken;
  String? deviceId;
  String? accessToken;
  String? refreshToken;
  String? accountId;

  // Cursor returned by the last cursor-aware alerts response. The caller can
  // persist it alongside its local inbox and pass it back to fetchAlerts.
  String? _lastAlertsCursor;
  String? get lastAlertsCursor => _lastAlertsCursor;

  void clearAlertsCursor() => _lastAlertsCursor = null;

  RelayApiClient({
    required this.baseUrl,
    http.Client? client,
    FlutterSecureStorage? secureStorage,
  })  : client = client ?? http.Client(),
        secureStorage = secureStorage ?? const FlutterSecureStorage();

  static const _sessionKeys = <String, String>{
    'deviceToken': 'relay.deviceToken',
    'deviceId': 'relay.deviceId',
    'accessToken': 'relay.accessToken',
    'refreshToken': 'relay.refreshToken',
    'accountId': 'relay.accountId',
    'accessExpiresAt': 'relay.accessExpiresAt',
  };

  Future<void>? _refreshInFlight;
  DateTime? _accessExpiresAt;

  /// Rotates the short-lived access token once. Concurrent callers share the
  /// same future, so a refresh-token family can never be consumed twice.
  Future<void> refreshAccessToken() => _refreshAccessToken();

  Future<void> _refreshAccessToken({String? failedAccessToken}) {
    final current = accessToken;
    if (failedAccessToken != null && current != null && current != failedAccessToken) {
      return Future.value();
    }
    final inFlight = _refreshInFlight;
    if (inFlight != null) return inFlight;
    final future = _performRefresh(failedAccessToken: failedAccessToken);
    _refreshInFlight = future;
    future.then<void>((_) {
      if (identical(_refreshInFlight, future)) _refreshInFlight = null;
    }, onError: (Object _, StackTrace __) {
      if (identical(_refreshInFlight, future)) _refreshInFlight = null;
    });
    return future;
  }

  Future<void> _performRefresh({String? failedAccessToken}) async {
    final token = refreshToken;
    if (token == null || token.isEmpty) {
      throw const RelayApiException(401, '登录已过期，请重新授权设备');
    }
    // A request may have been queued while another caller rotated the token.
    if (failedAccessToken != null && accessToken != null && accessToken != failedAccessToken) return;
    try {
      final response = await client.post(_uri('/api/v1/auth/refresh'),
          headers: const {'Accept': 'application/json', 'Content-Type': 'application/json'},
          body: jsonEncode({'refreshToken': token}));
      final value = _decode(response);
      if (value is! Map || value['accessToken'] is! String || value['refreshToken'] is! String) {
        throw const FormatException('服务器返回了无效的刷新凭证');
      }
      accessToken = value['accessToken'] as String;
      refreshToken = value['refreshToken'] as String;
      accountId = value['accountId'] as String? ?? accountId;
      final expiry = value['accessExpiresAt'];
      _accessExpiresAt = expiry is String ? DateTime.tryParse(expiry)?.toUtc() : null;
      await _persistSessionSafely();
    } on RelayApiException {
      await clearSession();
      rethrow;
    } catch (_) {
      rethrow;
    }
  }

  @override
  Future<void> restoreSession() async {
    try {
      deviceToken = await secureStorage.read(key: _sessionKeys['deviceToken']!);
      deviceId = await secureStorage.read(key: _sessionKeys['deviceId']!);
      accessToken = await secureStorage.read(key: _sessionKeys['accessToken']!);
      refreshToken = await secureStorage.read(key: _sessionKeys['refreshToken']!);
      accountId = await secureStorage.read(key: _sessionKeys['accountId']!);
      final expires = await secureStorage.read(key: _sessionKeys['accessExpiresAt']!);
      _accessExpiresAt = expires == null ? null : DateTime.tryParse(expires)?.toUtc();
      if (deviceToken == null || deviceId == null) {
        await clearSession();
      }
    } catch (_) {
      deviceToken = null;
      deviceId = null;
      accessToken = null;
      refreshToken = null;
      accountId = null;
      _accessExpiresAt = null;
    }
  }

  @override
  Future<void> clearSession() async {
    deviceToken = null;
    deviceId = null;
    accessToken = null;
    refreshToken = null;
    accountId = null;
    _accessExpiresAt = null;
    _refreshInFlight = null;
    for (final key in _sessionKeys.values) {
      try {
        await secureStorage.delete(key: key);
      } catch (_) {
        // Clearing in-memory credentials remains safe if the plugin is absent.
      }
    }
  }

  Future<void> _persistSession() async {
    final values = <String, String?>{
      'deviceToken': deviceToken,
      'deviceId': deviceId,
      'accessToken': accessToken,
      'refreshToken': refreshToken,
      'accountId': accountId,
      'accessExpiresAt': _accessExpiresAt?.toUtc().toIso8601String(),
    };
    for (final entry in values.entries) {
      final value = entry.value;
      final key = _sessionKeys[entry.key]!;
      if (value == null || value.isEmpty) {
        await secureStorage.delete(key: key);
      } else {
        await secureStorage.write(key: key, value: value);
      }
    }
  }

  Uri _uri(String path, [Map<String, String>? query]) {
    final base = Uri.parse(baseUrl);
    final pathUri = Uri.parse(path);
    return base.replace(
      path:
          '${base.path.replaceFirst(RegExp(r'/$'), '')}/${pathUri.path.replaceFirst(RegExp(r'^/'), '')}',
      queryParameters: {
        ...base.queryParameters,
        ...pathUri.queryParameters,
        if (query != null) ...query,
      },
    );
  }

  Map<String, String> get _headers => {
        'Accept': 'application/json',
        'Content-Type': 'application/json',
        if (accessToken != null || deviceToken != null)
          'Authorization': 'Bearer ${accessToken ?? deviceToken}',
        if (deviceId != null) 'X-Device-Id': deviceId!,
      };

  Future<http.Response> _authorized(Future<http.Response> Function() send,
      {bool retry = true}) async {
    final sentAccess = accessToken;
    final response = await send();
    if (response.statusCode != 401 || !retry || refreshToken == null) return response;
    await _refreshAccessToken(failedAccessToken: sentAccess);
    return _authorized(send, retry: false);
  }

  Future<dynamic> _post(String path, Map<String, dynamic> body) async {
    final response = await _authorized(() => client.post(_uri(path), headers: _headers, body: jsonEncode(body)));
    return _decode(response);
  }

  Future<dynamic> _put(String path, Map<String, dynamic> body) async {
    final response = await _authorized(() => client.put(_uri(path), headers: _headers, body: jsonEncode(body)));
    return _decode(response);
  }

  Future<http.Response> _get(String path, [Map<String, String>? query]) =>
      _authorized(() => client.get(_uri(path, query), headers: _headers));

  /// Exposes the same guarded transport for realtime HTTPS fallback calls.
  Future<http.Response> getAuthorized(String path, {Map<String, String>? query, http.Client? client}) =>
      _authorized(() => (client ?? this.client).get(_uri(path, query), headers: _headers));

  Future<http.Response> postAuthorized(String path, Map<String, dynamic> body, {http.Client? client}) =>
      _authorized(() => (client ?? this.client).post(_uri(path), headers: _headers, body: jsonEncode(body)));

  dynamic _decode(http.Response response) {
    dynamic body;
    if (response.body.isNotEmpty) {
      try {
        body = jsonDecode(response.body);
      } on FormatException {
        body = response.body;
      }
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      if (body is Map) {
        final map = Map<String, dynamic>.from(body);
        final nested = map['error'];
        if (nested is Map && nested['message'] is String) {
          throw RelayApiException(response.statusCode, nested['message'] as String,
              details: map);
        }
        if (map['message'] is String) {
          throw RelayApiException(response.statusCode, map['message'] as String, details: map);
        }
      }
      throw RelayApiException(response.statusCode, 'Server request failed');
    }
    return body;
  }

  @override
  Future<void> authorizeDevice({required String account, required String deviceName}) async {
    final identity = account.trim();
    final name = deviceName.trim();
    if (identity.isEmpty || name.isEmpty) {
      throw const FormatException('Account and device name are required');
    }
    // The server requires a verified access token for this account in
    // production. Do not send provider/subject values from a text field: they
    // are identity assertions, not credentials, and must come from OAuth.
    // This request deliberately never includes a password or SSO secret.
    final start = await _post('/api/v1/auth/device/start', {
      'accountId': identity,
      'deviceName': name,
      'deviceType': 'mobile',
    });
    if (start is! Map || start['challenge'] is! String) {
      throw const FormatException('Server returned an invalid authorization challenge');
    }
    final complete = await _post('/api/v1/auth/device/complete', {'challenge': start['challenge']});
    if (complete is! Map || complete['deviceToken'] is! String || complete['deviceId'] is! String) {
      throw const FormatException('Server returned invalid device credentials');
    }
    deviceToken = complete['deviceToken'] as String;
    deviceId = complete['deviceId'] as String;
    accessToken = complete['accessToken'] as String?;
    refreshToken = complete['refreshToken'] as String?;
    accountId = complete['accountId'] as String?;
    final expires = complete['accessExpiresAt'];
    _accessExpiresAt = expires is String ? DateTime.tryParse(expires)?.toUtc() : null;
    await _persistSessionSafely();
  }

  Future<void> _persistSessionSafely() async {
    // Credential persistence is best-effort on unsupported test/web platforms;
    // authorization itself must still succeed and never fall back to passwords.
    try {
      await _persistSession();
    } catch (_) {
      // Secure storage can be unavailable before a native plugin is registered.
    }
  }

  @override
  Future<void> login(String account, String password) async {
    throw UnsupportedError('Use device authorization; passwords are not accepted');
  }

  @override
  Future<String> createPairingCode() async {
    final value = await _post('/api/v1/pair', {});
    if (value is! Map || value['code'] is! String) {
      throw const FormatException(
          'Server returned an invalid pairing response');
    }
    return value['code'] as String;
  }

  @override
  Future<void> confirmPairing(String code) async {
    final value = await _post('/api/v1/pair/confirm', {
      'code': code,
      'deviceName': 'Alice-EVE mobile',
      'deviceType': 'mobile',
    });
    if (value is! Map ||
        value['deviceToken'] is! String ||
        value['deviceId'] is! String) {
      throw const FormatException('Server returned invalid device credentials');
    }
    deviceToken = value['deviceToken'] as String;
    deviceId = value['deviceId'] as String;
    accessToken = value['accessToken'] as String?;
    refreshToken = value['refreshToken'] as String?;
    accountId = value['accountId'] as String?;
    final expires = value['accessExpiresAt'];
    _accessExpiresAt = expires is String ? DateTime.tryParse(expires)?.toUtc() : null;
    await _persistSessionSafely();
  }

  @override
  Future<List<AlertEvent>> fetchAlerts({String? cursor, int? limit}) async {
    final effectiveCursor = cursor ?? _lastAlertsCursor;
    final query = <String, String>{
      if (deviceId != null) 'deviceId': deviceId!,
      if (effectiveCursor != null && effectiveCursor.isNotEmpty)
        'cursor': effectiveCursor,
      if (limit != null) 'limit': '$limit',
    };
    final response =
        await _get('/api/v1/alerts', query);
    final decoded = _decode(response);
    final List<dynamic> rawItems;
    if (decoded is List) {
      // Legacy list responses did not carry a cursor.
      rawItems = decoded;
    } else if (decoded is Map) {
      final responseMap = Map<String, dynamic>.from(decoded);
      final responseCursor = responseMap['cursor'];
      if (responseCursor != null) {
        _lastAlertsCursor = responseCursor.toString();
      }
      final items = responseMap['messages'] ??
          responseMap['events'] ??
          responseMap['items'] ??
          responseMap['backlog'];
      rawItems = items is List ? items : const <dynamic>[];
    } else {
      rawItems = const <dynamic>[];
    }
    return rawItems.map((item) {
      if (item is! Map) {
        throw const FormatException('Invalid event in server backlog');
      }
      return AlertEvent.fromEnvelope(
          MessageEnvelope.fromJson(Map<String, dynamic>.from(item)));
    }).toList(growable: false);
  }

  @override
  Stream<AlertEvent> watchAlerts() async* {
    final request = http.Request('GET', _uri('/api/v1/events'))
      ..headers.addAll(_headers);
    final response = await client.send(request);
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw RelayApiException(
          response.statusCode, 'Unable to open event stream');
    }
    final lines =
        response.stream.transform(utf8.decoder).transform(const LineSplitter());
    final buffer = <String>[];
    await for (final line in lines) {
      if (line.isEmpty) {
        final data = buffer
            .where((entry) => entry.startsWith('data:'))
            .map((entry) => entry.substring(5).trim())
            .join('\n');
        buffer.clear();
        if (data.isEmpty) continue;
        final value = jsonDecode(data);
        if (value is Map) {
          final envelope =
              MessageEnvelope.fromJson(Map<String, dynamic>.from(value));
          if (envelope.type == 'intel.alert' ||
              envelope.type == 'intel.summary' ||
              envelope.type == 'market.alert') {
            yield AlertEvent.fromEnvelope(envelope);
          }
        }
      } else if (line.startsWith('data:')) {
        buffer.add(line);
      }
    }
  }

  @override
  Future<AcknowledgeResult> acknowledgeAlert(String id) async {
    if (id.trim().isEmpty) {
      throw ArgumentError.value(id, 'id', 'must not be empty');
    }
    final value = await _post('/api/v1/messages/ack', {
      // `id` is retained for compatibility with the current server endpoint.
      'id': id,
      'deviceId': deviceId,
      'ackMessageId': id,
      'status': 'accepted',
      'stage': 'received',
      'receivedAt': DateTime.now().toUtc().toIso8601String(),
    });
    return AcknowledgeResult.fromJson(
        value is Map ? Map<String, dynamic>.from(value) : const {}, id);
  }

  @override
  Future<ReminderPreferencesDocument> fetchReminderPreferences() async {
    final response = await _get('/api/v1/notifications/preferences');
    final value = _decode(response);
    if (value is! Map) throw const FormatException('Invalid reminder preferences response');
    return ReminderPreferencesDocument.fromJson(Map<String, dynamic>.from(value));
  }

  @override
  Future<ReminderPreferencesDocument> updateReminderPreferences(ReminderPreferences preferences, {required int baseVersion, required String ifMatch}) async {
    final value = await _put('/api/v1/notifications/preferences', {
      'preferences': preferences.toJson(), 'baseVersion': baseVersion, 'ifMatch': ifMatch,
    });
    if (value is! Map) throw const FormatException('Invalid reminder preferences response');
    return ReminderPreferencesDocument.fromJson(Map<String, dynamic>.from(value));
  }

  @override
  Future<List<RegisteredPushToken>> fetchPushTokens() async {
    final value = _decode(await _get('/api/v1/notifications/tokens'));
    if (value is! List) throw const FormatException('Invalid push token list response');
    return value.map((item) {
      if (item is! Map) throw const FormatException('Invalid push token entry');
      return RegisteredPushToken.fromJson(Map<String, dynamic>.from(item));
    }).toList(growable: false);
  }

  @override
  Future<RegisteredPushToken> registerPushToken(PushTokenRegistration registration) async {
    final value = await _post('/api/v1/notifications/tokens', registration.toJson());
    if (value is! Map) throw const FormatException('Invalid push token response');
    return RegisteredPushToken.fromJson(Map<String, dynamic>.from(value));
  }

  @override
  Future<void> revokePushToken(String tokenId) async {
    final response = await _authorized(() => client.delete(_uri('/api/v1/notifications/tokens', {'id': tokenId}), headers: _headers));
    _decode(response);
  }

  @override
  Future<List<AccountDevice>> fetchDevices() async {
    final value = _decode(await _get('/api/v1/account/devices'));
    if (value is! Map || value['devices'] is! List) {
      throw const FormatException('Invalid device list response');
    }
    return (value['devices'] as List).map((item) {
      if (item is! Map) throw const FormatException('Invalid device entry');
      return AccountDevice.fromJson(Map<String, dynamic>.from(item), currentId: deviceId);
    }).toList(growable: false);
  }

  @override
  Future<void> revokeDevice(String targetDeviceId) async {
    if (targetDeviceId.trim().isEmpty) throw ArgumentError.value(targetDeviceId, 'deviceId');
    _decode(await _authorized(() => client.post(_uri('/api/v1/account/devices/revoke'), headers: _headers, body: jsonEncode({'deviceId': targetDeviceId}))));
    if (targetDeviceId == deviceId) await clearSession();
  }

  @override
  Future<void> revokeAllDevices() async {
    _decode(await _authorized(() => client.post(_uri('/api/v1/account/devices/revoke-all'), headers: _headers, body: jsonEncode({}))));
    await clearSession();
  }

  @override
  Future<void> sendReadOnlyRequest(
      String type, Map<String, dynamic> payload) async {
    final now = DateTime.now().toUtc();
    await _post('/api/v1/messages', {
      'v': 1,
      'id': _messageId(now),
      'type': type,
      'sender': 'mobile',
      'ts': now.toIso8601String(),
      'payload': payload,
      if (deviceId != null) 'deviceId': deviceId,
    });
  }

  String _messageId(DateTime now) =>
      'mobile-${now.microsecondsSinceEpoch}-${now.millisecond}';
}
