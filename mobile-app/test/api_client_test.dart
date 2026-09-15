import 'dart:convert';

import 'package:eve_assistant_mobile/api/api_client.dart';
import 'package:eve_assistant_mobile/models/protocol.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

class _MemorySecureStorage extends FlutterSecureStorage {
  final values = <String, String>{};
  @override
  Future<String?> read({required String key, IOSOptions? iOptions, AndroidOptions? aOptions, LinuxOptions? lOptions, WebOptions? webOptions, MacOsOptions? mOptions, WindowsOptions? wOptions}) async => values[key];
  @override
  Future<void> write({required String key, required String? value, IOSOptions? iOptions, AndroidOptions? aOptions, LinuxOptions? lOptions, WebOptions? webOptions, MacOsOptions? mOptions, WindowsOptions? wOptions}) async { if (value == null) { values.remove(key); } else { values[key] = value; } }
  @override
  Future<void> delete({required String key, IOSOptions? iOptions, AndroidOptions? aOptions, LinuxOptions? lOptions, WebOptions? webOptions, MacOsOptions? mOptions, WindowsOptions? wOptions}) async => values.remove(key);
}

void main() {
  test('confirmPairing stores device credentials and fetches owned backlog',
      () async {
    final requests = <http.Request>[];
    final client = MockClient((request) async {
      requests.add(request);
      if (request.url.path.endsWith('/pair/confirm')) {
        return http.Response(
            jsonEncode({'deviceId': 'mobile-1', 'deviceToken': 'token-1'}),
            200);
      }
      if (request.url.path.endsWith('/alerts')) {
        expect(request.headers['authorization'], 'Bearer token-1');
        expect(request.headers['x-device-id'], 'mobile-1');
        expect(request.url.queryParameters['deviceId'], 'mobile-1');
        expect(request.url.queryParameters['cursor'], 'cursor-1');
        return http.Response(
            jsonEncode({
              'messages': [
                {
                  'v': 1,
                  'id': 'msg-1',
                  'type': 'intel.alert',
                  'sender': 'desktop',
                  'ts': '2026-08-23T10:20:00Z',
                  'payload': {
                    'eventId': 'evt-1',
                    'title': 'Hostile in Jita',
                    'summary': 'Three pilots',
                    'severity': 'high',
                    'systemId': 30000142,
                    'systemName': 'Jita',
                    'observedAt': '2026-08-23T10:18:00Z',
                    'evidenceCount': 2,
                  },
                },
              ],
              'cursor': 42,
            }),
            200);
      }
      if (request.url.path.endsWith('/messages/ack')) {
        final body = jsonDecode(request.body) as Map<String, dynamic>;
        expect(body['ackMessageId'], 'msg-1');
        expect(body['stage'], 'received');
        expect(body['deviceId'], 'mobile-1');
        return http.Response(jsonEncode({'acked': true, 'id': 'msg-1'}), 200);
      }
      return http.Response('{}', 404);
    });

    final api =
        RelayApiClient(baseUrl: 'https://relay.example', client: client);
    await api.confirmPairing('123456');
    final alerts = await api.fetchAlerts(cursor: 'cursor-1', limit: 20);
    final ack = await api.acknowledgeAlert(alerts.single.id);

    expect(api.deviceId, 'mobile-1');
    expect(api.lastAlertsCursor, '42');
    expect(alerts.single.id, 'msg-1');
    expect(alerts.single.eventId, 'evt-1');
    expect(alerts.single.systemName, 'Jita');
    expect(alerts.single.evidenceCount, 2);
    expect(ack.status, 'accepted');
    expect(requests, hasLength(3));
  });

  test('authorizeDevice sends account id without password or arbitrary identity', () async {
    final requests = <http.Request>[];
    final client = MockClient((request) async {
      requests.add(request);
      return http.Response(jsonEncode({
        'error': {'code': 'identity_required', 'message': 'verified identity is required'}
      }), 401);
    });
    final api = RelayApiClient(baseUrl: 'https://relay.example', client: client);
    await expectLater(
      api.authorizeDevice(account: 'acct-1', deviceName: 'phone'),
      throwsA(isA<RelayApiException>().having((e) => e.statusCode, 'status', 401)),
    );
    final body = jsonDecode(requests.single.body) as Map<String, dynamic>;
    expect(body['accountId'], 'acct-1');
    expect(body.containsKey('provider'), isFalse);
    expect(body.containsKey('subject'), isFalse);
    expect(body.containsKey('password'), isFalse);
  });

  test('secure session can be restored and cleared', () async {
    final storage = _MemorySecureStorage();
    storage.values['relay.deviceToken'] = 'token-1';
    storage.values['relay.deviceId'] = 'mobile-1';
    storage.values['relay.refreshToken'] = 'refresh-1';
    final api = RelayApiClient(baseUrl: 'https://relay.example', secureStorage: storage);
    await api.restoreSession();
    expect(api.deviceToken, 'token-1');
    expect(api.deviceId, 'mobile-1');
    expect(api.refreshToken, 'refresh-1');
    await api.clearSession();
    expect(api.deviceToken, isNull);
    expect(storage.values, isEmpty);
  });

  test('fetchAlerts reuses and clears the saved cursor', () async {
    final cursors = <String?>[];
    final client = MockClient((request) async {
      cursors.add(request.url.queryParameters['cursor']);
      return http.Response(
          jsonEncode({
            'messages': [
              {
                'v': 1,
                'id': 'msg-${cursors.length}',
                'type': 'intel.alert',
                'sender': 'desktop',
                'ts': '2026-08-23T10:20:00Z',
                'payload': {
                  'title': 'Alert',
                  'summary': 'Summary',
                  'severity': 'normal'
                },
              },
            ],
            'cursor': cursors.length,
          }),
          200);
    });
    final api =
        RelayApiClient(baseUrl: 'https://relay.example', client: client);

    await api.fetchAlerts();
    await api.fetchAlerts();
    expect(cursors, [null, '1']);
    expect(api.lastAlertsCursor, '2');

    api.clearAlertsCursor();
    await api.fetchAlerts();
    expect(cursors, [null, '1', null]);
  });

  test('SSE stream parses alert data frames', () async {
    final client = MockClient((request) async {
      expect(request.url.path, '/api/v1/events');
      expect(request.headers['authorization'], 'Bearer token');
      return http.Response(
        'data: {"v":1,"id":"stream-1","type":"intel.alert","sender":"desktop","ts":"2026-08-23T10:20:00Z","payload":{"title":"Stream alert","summary":"Live","severity":"high"}}\n\n',
        200,
        headers: {'content-type': 'text/event-stream'},
      );
    });
    final api = RelayApiClient(baseUrl: 'https://relay.example', client: client)
      ..deviceToken = 'token';
    expect((await api.watchAlerts().toList()).single.title, 'Stream alert');
  });

  test('envelope supports protocol and relay wire field names', () {
    final envelope = MessageEnvelope.fromJson({
      'version': 1,
      'messageId': 'msg-2',
      'sender': 'desktop',
      'type': 'intel.alert',
      'createdAt': '2026-08-23T10:20:00Z',
      'payload': {'title': 'Alert', 'summary': 'Summary', 'severity': 'normal'},
    });
    expect(envelope.toRelayJson()['v'], 1);
    expect(envelope.toRelayJson()['id'], 'msg-2');
    expect(AlertEvent.fromEnvelope(envelope).title, 'Alert');

    expect(
      () => MessageEnvelope.fromJson({...envelope.toRelayJson(), 'v': 2}),
      throwsFormatException,
    );
  });
}
