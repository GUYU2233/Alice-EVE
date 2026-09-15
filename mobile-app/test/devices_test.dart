import 'dart:convert';

import 'package:eve_assistant_mobile/api/api_client.dart';
import 'package:eve_assistant_mobile/devices/device_management_page.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  testWidgets('device page renders devices and current badge', (tester) async {
    final client = MockClient((request) async {
      if (request.url.path.endsWith('/account/devices')) {
        return http.Response(jsonEncode({'devices': [
          {'id': 'mobile-1', 'name': 'My Phone', 'type': 'mobile', 'lastActiveAt': DateTime.now().toIso8601String(), 'current': true},
          {'id': 'desktop-1', 'name': 'Desktop', 'type': 'desktop', 'lastActiveAt': DateTime.now().subtract(const Duration(hours: 2)).toIso8601String(), 'current': false},
        ]}), 200);
      }
      return http.Response('{}', 204);
    });
    final api = RelayApiClient(baseUrl: 'https://relay.example', client: client)
      ..deviceToken = 'token'
      ..deviceId = 'mobile-1';
    await tester.pumpWidget(MaterialApp(home: DeviceManagementPage(api: api)));
    await tester.pumpAndSettle();
    expect(find.text('My Phone'), findsOneWidget);
    expect(find.text('Desktop'), findsOneWidget);
    expect(find.text('当前设备'), findsOneWidget);
    expect(find.text('撤销全部设备'), findsOneWidget);
  });
}
