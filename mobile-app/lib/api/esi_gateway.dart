import 'dart:convert';
import 'dart:async';
import 'package:http/http.dart' as http;

enum GatewayErrorKind { timeout, network, http, parse }

class GatewayException implements Exception {
  final GatewayErrorKind kind; final String message; final int? statusCode;
  GatewayException(this.kind, this.message, {this.statusCode});
  @override String toString() => 'GatewayException($kind): $message';
}

class EsiGateway {
  final String baseUrl; final http.Client client; final Duration timeout; final String userAgent;
  final Map<String, _CacheEntry> _cache = {};
  EsiGateway({required this.baseUrl, http.Client? client, this.timeout = const Duration(seconds: 10), this.userAgent = 'Alice-EVE/0.1 (+https://github.com/alice-eve)'}) : client = client ?? http.Client();

  Future<dynamic> getJson(String path) async {
    final uri = Uri.parse('$baseUrl$path'); final old = _cache[path];
    final headers = <String,String>{'User-Agent': userAgent, 'Accept':'application/json'};
    if (old?.etag != null) headers['If-None-Match'] = old!.etag!;
    http.Response response;
    try { response = await client.get(uri, headers: headers).timeout(timeout); }
    on TimeoutException { throw GatewayException(GatewayErrorKind.timeout, 'request timed out'); }
    on http.ClientException catch (e) { throw GatewayException(GatewayErrorKind.network, e.message); }
    if (response.statusCode == 304 && old != null) return old.value;
    if (response.statusCode < 200 || response.statusCode >= 300) throw GatewayException(GatewayErrorKind.http, 'HTTP ${response.statusCode}', statusCode: response.statusCode);
    try { final value = jsonDecode(response.body); _cache[path] = _CacheEntry(value, response.headers['etag']); return value; }
    catch (e) { throw GatewayException(GatewayErrorKind.parse, 'invalid JSON: $e'); }
  }
  void clearCache() => _cache.clear();
}
class _CacheEntry { final dynamic value; final String? etag; _CacheEntry(this.value, this.etag); }
