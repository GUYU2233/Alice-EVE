import '../models/notifications.dart';

/// Platform-neutral local notification port. Android/iOS adapters can bridge
/// this to flutter_local_notifications without coupling business logic to a
/// plugin (and tests can use [MemoryNotificationPresenter]).
abstract interface class NotificationPresenter {
  Future<void> show({required String title, required String body, Map<String, dynamic> data = const {}});
}

class MemoryNotificationPresenter implements NotificationPresenter {
  final List<PresentedNotification> notifications = [];
  @override
  Future<void> show({required String title, required String body, Map<String, dynamic> data = const {}}) async {
    notifications.add(PresentedNotification(title: title, body: body, data: Map<String, dynamic>.from(data)));
  }
}

class PresentedNotification {
  final String title;
  final String body;
  final Map<String, dynamic> data;
  const PresentedNotification({required this.title, required this.body, required this.data});
}

/// Shared background-message translation. Firebase Messaging adapters should
/// call [handleMessage] from their top-level background callback.
class BackgroundMessageHandler {
  final NotificationPresenter presenter;
  final ReminderPreferences preferences;
  const BackgroundMessageHandler({required this.presenter, this.preferences = const ReminderPreferences()});

  Future<void> handleMessage(Map<String, dynamic> message) async {
    if (!preferences.enabled) return;
    final nestedData = message['data'];
    final type = message['type'] as String? ??
        (nestedData is Map ? nestedData['type'] as String? : null);
    if (type == 'intel.alert' && !preferences.intelAlerts || type == 'market.alert' && !preferences.marketAlerts || type == 'conversation.update' && !preferences.conversationUpdates) return;
    final notification = message['notification'];
    final title = notification is Map ? notification['title'] as String? : message['title'] as String?;
    final body = notification is Map ? notification['body'] as String? : message['body'] as String?;
    if (title == null || body == null || title.isEmpty) return;
    await presenter.show(title: title, body: body, data: Map<String, dynamic>.from(message['data'] is Map ? message['data'] as Map : const {}));
  }
}
