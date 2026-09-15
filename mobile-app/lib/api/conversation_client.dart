import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;
import 'package:web_socket_channel/web_socket_channel.dart';

import 'api_client.dart';

/// A resilient conversation transport. WSS is preferred while the app is
/// foregrounded; HTTPS sync fills gaps after reconnects and on platforms where
/// WebSockets are unavailable.
class ConversationEvent {
  final String type;
  final int cursor;
  final Map<String, dynamic> payload;
  final String? id;
  const ConversationEvent({required this.type, required this.cursor, required this.payload, this.id});

  factory ConversationEvent.fromJson(Map<String, dynamic> json) {
    final raw = json['event'] is Map ? Map<String, dynamic>.from(json['event'] as Map) : json;
    final payload = raw['payload'];
    return ConversationEvent(
      type: raw['type'] as String? ?? json['type'] as String? ?? 'unknown',
      cursor: _int(json['cursor'] ?? raw['cursor'] ?? 0),
      payload: payload is Map ? Map<String, dynamic>.from(payload) : raw,
      id: raw['id'] as String? ?? raw['messageId'] as String?,
    );
  }
  static int _int(dynamic value) => value is int ? value : int.tryParse('$value') ?? 0;
}

class ConversationMessage {
  final String id;
  final String conversationId;
  final String body;
  final String? operation;
  final int? cursor;
  const ConversationMessage({required this.id, required this.conversationId, required this.body, this.operation, this.cursor});

  factory ConversationMessage.fromJson(Map<String, dynamic> json) => ConversationMessage(
    id: (json['id'] ?? json['messageId'] ?? '').toString(),
    conversationId: (json['conversationId'] ?? '').toString(),
    body: (json['body'] ?? '').toString(),
    operation: json['operation'] as String?,
    cursor: json['cursor'] == null ? null : ConversationEvent._int(json['cursor']),
  );
}

class ConversationClient {
  final RelayApiClient api;
  final String conversationId;
  final http.Client httpClient;
  final WebSocketChannel Function(Uri uri, Map<String, String> headers)? channelFactory;
  final Duration reconnectDelay;
  final Duration pingInterval;
  final _events = StreamController<ConversationEvent>.broadcast();
  WebSocketChannel? _channel;
  StreamSubscription? _socketSubscription;
  Timer? _pingTimer;
  Timer? _reconnectTimer;
  bool _closed = false;
  bool _connecting = false;
  int _cursor = 0;

  ConversationClient({required this.api, required this.conversationId, http.Client? httpClient,
      this.channelFactory, this.reconnectDelay = const Duration(seconds: 2), this.pingInterval = const Duration(seconds: 25)}) : httpClient = httpClient ?? http.Client();

  int get cursor => _cursor;
  Stream<ConversationEvent> get events => _events.stream;
  bool get isConnected => _channel != null;

  Future<void> connect({int? cursor}) async {
    if (cursor != null && cursor > _cursor) _cursor = cursor;
    _closed = false;
    await _sync();
    await _openSocket();
  }

  Future<void> _openSocket() async {
    if (_closed || _connecting || _channel != null) return;
    _connecting = true;
    try {
      final base = Uri.parse(api.baseUrl);
      final scheme = base.scheme == 'https' ? 'wss' : 'ws';
      final credential = api.accessToken ?? api.deviceToken;
      final uri = base.replace(scheme: scheme, path: '${base.path.replaceFirst(RegExp(r'/$'), '')}/api/v1/realtime', queryParameters: {'cursor': '$_cursor', if (credential != null) 'access_token': credential});
      final headers = <String, String>{if (credential != null) 'Authorization': 'Bearer $credential'};
      final channel = channelFactory?.call(uri, headers) ?? WebSocketChannel.connect(uri);
      _channel = channel;
      await channel.ready;
      _socketSubscription = channel.stream.listen(_onSocketData, onError: (_) => _socketClosed(), onDone: _socketClosed, cancelOnError: true);
      _pingTimer?.cancel();
      _pingTimer = Timer.periodic(pingInterval, (_) => _send({'type': 'ping'}));
      _send({'type': 'subscribe', 'cursor': _cursor, 'after': _cursor});
    } catch (_) {
      _socketClosed();
    } finally {
      _connecting = false;
    }
  }

  void _onSocketData(dynamic data) {
    try {
      final decoded = data is String ? jsonDecode(data) : data;
      if (decoded is! Map) return;
      final frame = Map<String, dynamic>.from(decoded);
      final type = frame['type'] as String? ?? '';
      if (type == 'pong' || type == 'subscribed') {
        _advance(frame['cursor']);
        return;
      }
      if (type == 'event' && frame['event'] is Map) {
        final event = ConversationEvent.fromJson(frame);
        _advance(event.cursor);
        _events.add(event);
      }
    } catch (_) {
      // Malformed frames are ignored; the next valid frame remains processable.
    }
  }

  void _advance(dynamic value) {
    final next = ConversationEvent._int(value);
    if (next > _cursor) _cursor = next;
  }

  void _send(Map<String, dynamic> value) {
    try { _channel?.sink.add(jsonEncode(value)); } catch (_) { _socketClosed(); }
  }

  void _socketClosed() {
    _pingTimer?.cancel();
    _pingTimer = null;
    _socketSubscription?.cancel();
    _socketSubscription = null;
    final channel = _channel;
    _channel = null;
    try { channel?.sink.close(); } catch (_) {}
    if (!_closed) {
      _reconnectTimer?.cancel();
      _reconnectTimer = Timer(reconnectDelay, () async { await _sync(); await _openSocket(); });
    }
  }

  Future<void> _sync() async {
    if (_closed || api.deviceToken == null && api.accessToken == null) return;
    try {
      final response = await api.getAuthorized('/api/v1/sync', query: {'cursor': '$_cursor', 'limit': '100'}, client: httpClient);
      if (response.statusCode < 200 || response.statusCode >= 300) return;
      final body = jsonDecode(response.body);
      if (body is! Map) return;
      final messages = body['messages'] ?? body['items'] ?? body['events'];
      if (messages is List) {
        for (final item in messages) {
          if (item is! Map) continue;
          final event = ConversationEvent.fromJson(Map<String, dynamic>.from(item));
          _advance(item['cursor'] ?? event.cursor);
          _events.add(event);
        }
      }
      _advance(body['cursor']);
    } catch (_) {}
  }

  Future<List<Map<String, dynamic>>> listConversations() async {
    final response = await api.getAuthorized('/api/v1/agent/conversations', client: httpClient);
    final value = jsonDecode(response.body);
    if (value is! Map || value['conversations'] is! List) return const [];
    return (value['conversations'] as List).whereType<Map>().map((e) => Map<String, dynamic>.from(e)).toList(growable: false);
  }

  Future<ConversationClient> createConversation({required String targetDeviceId, String agentKind = 'eve-agent'}) async {
    final response = await api.postAuthorized('/api/v1/agent/conversations', {'targetDeviceId': targetDeviceId, 'agentKind': agentKind}, client: httpClient);
    final value = jsonDecode(response.body);
    if (value is! Map || value['id'] is! String) throw const FormatException('Invalid conversation response');
    return ConversationClient(api: api, conversationId: value['id'] as String, httpClient: httpClient, channelFactory: channelFactory, reconnectDelay: reconnectDelay, pingInterval: pingInterval);
  }

  Future<ConversationMessage> sendMessage({required String body, String? operation, String? clientMessageId}) async {
    final response = await api.postAuthorized('/api/v1/conversations/$conversationId/messages', {'body': body, if (operation != null) 'operation': operation, 'clientMessageId': clientMessageId ?? 'mobile-${DateTime.now().microsecondsSinceEpoch}'}, client: httpClient);
    final decoded = jsonDecode(response.body);
    if (decoded is! Map || decoded['message'] is! Map) throw const FormatException('Invalid conversation message response');
    return ConversationMessage.fromJson(Map<String, dynamic>.from(decoded['message'] as Map));
  }

  Future<void> close() async {
    _closed = true;
    _reconnectTimer?.cancel();
    _pingTimer?.cancel();
    await _socketSubscription?.cancel();
    try { await _channel?.sink.close(); } catch (_) {}
    _channel = null;
    await _events.close();
  }
}
