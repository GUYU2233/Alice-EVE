import 'package:eve_assistant_mobile/models/notifications.dart';
import 'package:eve_assistant_mobile/notifications/notification_service.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('background handler presents enabled alert and honors preferences', () async {
    final presenter = MemoryNotificationPresenter();
    final handler = BackgroundMessageHandler(presenter: presenter);
    await handler.handleMessage({'type': 'intel.alert', 'notification': {'title': 'Hostile', 'body': 'Jita'}, 'data': {'id': '1'}});
    expect(presenter.notifications.single.title, 'Hostile');
    expect(presenter.notifications.single.data['id'], '1');

    final disabled = BackgroundMessageHandler(presenter: presenter, preferences: const ReminderPreferences(intelAlerts: false));
    await disabled.handleMessage({'type': 'intel.alert', 'notification': {'title': 'Nope', 'body': 'Jita'}});
    expect(presenter.notifications, hasLength(1));
  });
}
