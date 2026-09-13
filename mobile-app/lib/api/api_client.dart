import 'dart:convert';
import 'package:http/http.dart' as http;
import '../models/protocol.dart';

abstract interface class ApiClient {
  Future<void> login(String account, String password);
  Future<String> createPairingCode();
  Future<void> confirmPairing(String code);
  Future<List<AlertEvent>> fetchAlerts();
  Future<void> acknowledgeAlert(String id);
  Future<void> sendReadOnlyRequest(String type, Map<String, dynamic> payload);
}

class RelayApiClient implements ApiClient {
  final String baseUrl;
  String? deviceToken;
  RelayApiClient({required this.baseUrl});
  Uri _uri(String path) => Uri.parse('$baseUrl$path');
  Map<String,String> get _headers => {'Content-Type':'application/json', if (deviceToken != null) 'Authorization':'Bearer $deviceToken'};
  Future<dynamic> _post(String path, Map<String,dynamic> body) async { final r=await http.post(_uri(path),headers:_headers,body:jsonEncode(body)); if(r.statusCode>=300) throw Exception(r.body); return jsonDecode(r.body); }
  @override Future<void> login(String account, String password) async {}
  @override Future<String> createPairingCode() async { final v=await _post('/api/v1/pair',{}); return v['code'] as String; }
  @override Future<void> confirmPairing(String code) async { final v=await _post('/api/v1/pair/confirm',{'code':code,'deviceName':'mobile','deviceType':'mobile'}); deviceToken=v['deviceToken'] as String; }
  @override Future<List<AlertEvent>> fetchAlerts() async {
    final r = await http.get(_uri('/api/v1/alerts'), headers: _headers);
    if (r.statusCode >= 300) throw Exception(r.body);
    final decoded = jsonDecode(r.body);
    final items = decoded is List ? decoded : (decoded['events'] as List? ?? const []);
    return items.map((item) {
      final map = Map<String, dynamic>.from(item as Map);
      return AlertEvent.fromEnvelope(MessageEnvelope.fromJson(map));
    }).toList();
  }
  @override Future<void> acknowledgeAlert(String id) async {
    await _post('/api/v1/messages/ack', {'id': id});
  }
  @override Future<void> sendReadOnlyRequest(String type, Map<String, dynamic> payload) async { await _post('/api/v1/messages',{'v':1,'id':DateTime.now().microsecondsSinceEpoch.toString(),'type':type,'sender':'mobile','ts':DateTime.now().toUtc().toIso8601String(),'payload':payload}); }
}
