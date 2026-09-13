class MessageEnvelope {
  final int version;
  final String messageId;
  final String sender;
  final String type;
  final DateTime createdAt;
  final Map<String, dynamic> payload;

  const MessageEnvelope({required this.version, required this.messageId, required this.sender, required this.type, required this.createdAt, required this.payload});

  factory MessageEnvelope.fromJson(Map<String, dynamic> json) => MessageEnvelope(
    version: json['v'] as int? ?? 1,
    messageId: json['id'] as String? ?? '',
    sender: json['sender'] as String? ?? '',
    type: json['type'] as String? ?? '',
    createdAt: DateTime.tryParse(json['ts'] as String? ?? '') ?? DateTime.now(),
    payload: Map<String, dynamic>.from(json['payload'] as Map? ?? {}),
  );

  Map<String, dynamic> toJson() => {'v': version, 'id': messageId, 'sender': sender, 'type': type, 'ts': createdAt.toUtc().toIso8601String(), 'payload': payload};
}

class AlertEvent {
  final String id;
  final String title;
  final String summary;
  final String severity;
  final int? systemId;
  final DateTime createdAt;

  const AlertEvent({required this.id, required this.title, required this.summary, required this.severity, this.systemId, required this.createdAt});

  factory AlertEvent.fromEnvelope(MessageEnvelope e) => AlertEvent(id: e.messageId, title: e.payload['title'] as String? ?? 'EVE 告警', summary: e.payload['summary'] as String? ?? '', severity: e.payload['severity'] as String? ?? 'info', systemId: e.payload['systemId'] as int?, createdAt: e.createdAt);
}
