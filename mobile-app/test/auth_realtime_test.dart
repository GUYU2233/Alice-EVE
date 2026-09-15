import 'dart:async';
import 'dart:convert';

import 'package:eve_assistant_mobile/api/api_client.dart';
import 'package:eve_assistant_mobile/api/conversation_client.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

class _Storage extends FlutterSecureStorage {
  final values = <String, String>{};
  @override
  Future<String?> read({required String key, IOSOptions? iOptions, AndroidOptions? aOptions, LinuxOptions? lOptions, WebOptions? webOptions, MacOsOptions? mOptions, WindowsOptions? wOptions}) async => values[key];
  @override
  Future<void> write({required String key, required String? value, IOSOptions? iOptions, AndroidOptions? aOptions, LinuxOptions? lOptions, WebOptions? webOptions, MacOsOptions? mOptions, WindowsOptions? wOptions}) async { if (value != null) values[key] = value; }
  @override
  Future<void> delete({required String key, IOSOptions? iOptions, AndroidOptions? aOptions, LinuxOptions? lOptions, WebOptions? webOptions, MacOsOptions? mOptions, WindowsOptions? wOptions}) async => values.remove(key);
}

void main() {
  test('401 requests share one refresh and retry with rotated access', () async {
    final storage = _Storage();
    final refreshGate = Completer<void>();
    var refreshCalls = 0;
    var dataCalls = 0;
    final client = MockClient((request) async {
      if (request.url.path.endsWith('/auth/refresh')) {
        refreshCalls++;
        await refreshGate.future;
        return http.Response(jsonEncode({'accessToken': 'access-2', 'refreshToken': 'refresh-2', 'accountId': 'acct', 'accessExpiresAt': '2030-01-01T00:00:00Z'}), 200);
      }
      dataCalls++;
      if (request.headers['authorization'] == 'Bearer access-1') return http.Response('{}', 401);
      expect(request.headers['authorization'], 'Bearer access-2');
      return http.Response(jsonEncode({'messages': [], 'cursor': 2}), 200);
    });
    final api = RelayApiClient(baseUrl: 'https://relay.example', client: client, secureStorage: storage)
      ..deviceToken = 'device-token'
      ..deviceId = 'mobile-1'
      ..accessToken = 'access-1'
      ..refreshToken = 'refresh-1';
    final first = api.fetchAlerts();
    final second = api.fetchAlerts();
    await Future<void>.delayed(Duration.zero);
    expect(refreshCalls, 1);
    refreshGate.complete();
    await Future.wait([first, second]);
    expect(dataCalls, 4); // two initial 401s + two replayed requests
    expect(api.accessToken, 'access-2');
    expect(api.refreshToken, 'refresh-2');
  });

  test('conversation event parser accepts realtime event frames', () {
    final event = ConversationEvent.fromJson({'type': 'event', 'cursor': 9, 'event': {'id': 'evt-9', 'type': 'conversation.message', 'payload': {'body': 'hello'}, 'cursor': 9}});
    expect(event.id, 'evt-9');
    expect(event.cursor, 9);
    expect(event.payload['body'], 'hello');
  });
}
