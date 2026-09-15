class AccountDevice {
  final String id;
  final String name;
  final String type;
  final DateTime? lastActiveAt;
  final bool current;

  const AccountDevice({
    required this.id,
    required this.name,
    required this.type,
    this.lastActiveAt,
    this.current = false,
  });

  factory AccountDevice.fromJson(Map<String, dynamic> json, {String? currentId}) {
    final rawLastActive = json['lastActiveAt'] ?? json['lastSeenAt'] ?? json['createdAt'];
    return AccountDevice(
      id: (json['id'] ?? json['deviceId'] ?? '').toString(),
      name: (json['name'] ?? json['deviceName'] ?? '未命名设备').toString(),
      type: (json['type'] ?? json['deviceType'] ?? 'unknown').toString(),
      lastActiveAt: rawLastActive is String ? DateTime.tryParse(rawLastActive) : null,
      current: json['current'] == true || (currentId != null && (json['id'] ?? json['deviceId']) == currentId),
    );
  }
}
