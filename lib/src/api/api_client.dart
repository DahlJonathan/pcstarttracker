import 'dart:convert';

import 'package:http/http.dart' as http;

import '../models/device.dart';
import '../models/device_event.dart';

/// Raised when the backend returns a non-2xx response.
class ApiException implements Exception {
  ApiException(this.statusCode, this.message);
  final int statusCode;
  final String message;
  @override
  String toString() => message;
}

/// Thin REST client for the pc-tracker backend.
class ApiClient {
  ApiClient({required this.baseUrl, this.token});

  /// Backend base URL, e.g. `http://10.0.2.2:8080` (Android emulator -> host).
  final String baseUrl;

  /// Bearer JWT for the signed-in user (null before login).
  String? token;

  Map<String, String> get _headers => {
    'Content-Type': 'application/json',
    if (token != null) 'Authorization': 'Bearer $token',
  };

  Uri _uri(String path) => Uri.parse('$baseUrl$path');

  /// Signs up a new user and returns the issued JWT.
  Future<String> signup(String email, String password) =>
      _auth('/api/v1/auth/signup', email, password);

  /// Logs in an existing user and returns the issued JWT.
  Future<String> login(String email, String password) =>
      _auth('/api/v1/auth/login', email, password);

  Future<String> _auth(String path, String email, String password) async {
    final res = await http.post(
      _uri(path),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({'email': email, 'password': password}),
    );
    final body = _decode(res);
    final t = body['token'] as String;
    token = t;
    return t;
  }

  /// Fetches the signed-in user's devices with computed status.
  Future<List<Device>> listDevices() async {
    final res = await http.get(_uri('/api/v1/devices'), headers: _headers);
    final body = _decode(res);
    final list = (body['devices'] as List<dynamic>? ?? const []);
    return list
        .map((e) => Device.fromJson(e as Map<String, dynamic>))
        .toList(growable: false);
  }

  /// Claims a device for the signed-in user via a scanned QR token or a code.
  Future<void> claimDevice({String? pairingToken, String? code}) async {
    final res = await http.post(
      _uri('/api/v1/pair/claim'),
      headers: _headers,
      body: jsonEncode({'pairing_token': ?pairingToken, 'code': ?code}),
    );
    _decode(res);
  }

  /// Unpairs (deletes) a device owned by the signed-in user.
  Future<void> deleteDevice(String id) async {
    final res = await http.delete(
      _uri('/api/v1/devices/$id'),
      headers: _headers,
    );
    if (res.statusCode == 204) return;
    _decode(res);
  }

  /// Fetches a device's boot/shutdown history, newest first.
  Future<List<DeviceEvent>> listDeviceEvents(String id) async {
    final res = await http.get(
      _uri('/api/v1/devices/$id/history'),
      headers: _headers,
    );
    final body = _decode(res);
    final list = (body['events'] as List<dynamic>? ?? const []);
    return list
        .map((e) => DeviceEvent.fromJson(e as Map<String, dynamic>))
        .toList(growable: false);
  }

  Map<String, dynamic> _decode(http.Response res) {
    final isJson = (res.headers['content-type'] ?? '').contains(
      'application/json',
    );
    final Map<String, dynamic> parsed = isJson && res.body.isNotEmpty
        ? jsonDecode(res.body) as Map<String, dynamic>
        : {};
    if (res.statusCode >= 200 && res.statusCode < 300) {
      return parsed;
    }
    throw ApiException(
      res.statusCode,
      (parsed['error'] as String?) ?? 'Request failed (${res.statusCode})',
    );
  }
}
