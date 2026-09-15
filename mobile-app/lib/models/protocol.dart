import 'dart:convert';

/// Versioned message envelope shared by the server and mobile clients.
class MessageEnvelope {
  final int version;
  final String messageId;
  final String? correlationId;
  final String? conversationId;
  final String sender;
  final String? recipient;
  final String type;
  final DateTime createdAt;
  final DateTime? expiresAt;
  final Map<String, dynamic> payload;
  final Map<String, dynamic> meta;

  const MessageEnvelope({
    required this.version,
    required this.messageId,
    this.correlationId,
    this.conversationId,
    required this.sender,
    this.recipient,
    required this.type,
    required this.createdAt,
    this.expiresAt,
    required this.payload,
    this.meta = const {},
  });

  factory MessageEnvelope.fromJson(Map<String, dynamic> json) {
    final version = json['version'] ?? json['v'];
    final id = json['messageId'] ?? json['id'];
    final created = json['createdAt'] ?? json['ts'];
    if (version is! int || version != 1) {
      throw const FormatException('Unsupported protocol envelope version');
    }
    if (id is! String || id.isEmpty || created is! String) {
      throw const FormatException('Invalid protocol envelope');
    }
    final payload = json['payload'];
    if (payload is! Map) {
      throw const FormatException('Envelope payload must be an object');
    }
    final parsedCreated = DateTime.tryParse(created);
    if (parsedCreated == null) {
      throw const FormatException('Invalid envelope createdAt');
    }
    return MessageEnvelope(
      version: version,
      messageId: id,
      correlationId: json['correlationId'] as String?,
      conversationId: json['conversationId'] as String?,
      sender: json['sender'] as String? ?? '',
      recipient: json['recipient'] as String?,
      type: json['type'] as String? ?? '',
      createdAt: parsedCreated.toUtc(),
      expiresAt: _date(json['expiresAt']),
      payload: Map<String, dynamic>.from(payload),
      meta: json['meta'] is Map
          ? Map<String, dynamic>.from(json['meta'] as Map)
          : const {},
    );
  }

  Map<String, dynamic> toJson() => {
        'version': version,
        'messageId': messageId,
        if (correlationId != null) 'correlationId': correlationId,
        if (conversationId != null) 'conversationId': conversationId,
        'sender': sender,
        if (recipient != null) 'recipient': recipient,
        'type': type,
        'createdAt': createdAt.toUtc().toIso8601String(),
        if (expiresAt != null)
          'expiresAt': expiresAt!.toUtc().toIso8601String(),
        'payload': payload,
        if (meta.isNotEmpty) 'meta': meta,
      };

  /// Accepts the compact field names emitted by the current Go server too.
  /// The `toRelayJson` method name remains for wire-compatibility.
  Map<String, dynamic> toRelayJson() => {
        'v': version,
        'id': messageId,
        'type': type,
        'ts': createdAt.toUtc().toIso8601String(),
        'sender': sender,
        'payload': payload,
      };

  String encode() => jsonEncode(toJson());

  static DateTime? _date(dynamic value) =>
      value is String ? DateTime.tryParse(value)?.toUtc() : null;
}

class AlertEvent {
  /// The persisted envelope id. ACKs must always use this value, not payload eventId.
  final String id;
  final String? eventId;
  final String title;
  final String summary;
  final String severity;
  final int? systemId;
  final String? systemName;
  final String? source;
  final DateTime createdAt;
  final DateTime? observedAt;
  final DateTime? expiresAt;
  final int? evidenceCount;

  const AlertEvent({
    required this.id,
    this.eventId,
    required this.title,
    required this.summary,
    required this.severity,
    this.systemId,
    this.systemName,
    this.source,
    required this.createdAt,
    this.observedAt,
    this.expiresAt,
    this.evidenceCount,
  });

  factory AlertEvent.fromEnvelope(MessageEnvelope envelope) {
    if (envelope.type != 'intel.alert' &&
        envelope.type != 'intel.summary' &&
        envelope.type != 'market.alert') {
      throw FormatException('Not an alert envelope: ${envelope.type}');
    }
    final p = envelope.payload;
    return AlertEvent(
      id: envelope.messageId,
      eventId: p['eventId'] as String?,
      title: p['title'] as String? ?? 'EVE 告警',
      summary: p['summary'] as String? ?? '',
      severity: p['severity'] as String? ?? 'normal',
      systemId: _int(p['systemId']),
      systemName: p['systemName'] as String?,
      source: p['source'] as String?,
      createdAt: envelope.createdAt,
      observedAt: _date(p['observedAt']),
      expiresAt: _date(p['expiresAt']) ?? envelope.expiresAt,
      evidenceCount: _int(p['evidenceCount']),
    );
  }

  static int? _int(dynamic value) =>
      value is int ? value : int.tryParse('$value');
  static DateTime? _date(dynamic value) =>
      value is String ? DateTime.tryParse(value)?.toUtc() : null;
}

class AcknowledgeResult {
  final String messageId;
  final String status;
  final String stage;

  const AcknowledgeResult(
      {required this.messageId, required this.status, required this.stage});

  factory AcknowledgeResult.fromJson(
          Map<String, dynamic> json, String fallbackId) =>
      AcknowledgeResult(
        messageId: (json['messageId'] ?? json['id'] ?? fallbackId) as String,
        status: json['status'] as String? ??
            (json['acked'] == true ? 'accepted' : 'failed'),
        stage: json['stage'] as String? ?? 'received',
      );
}
