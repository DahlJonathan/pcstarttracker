import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../api/api_client.dart';
import '../models/device.dart';

/// Default backend URL. On the Android emulator, 10.0.2.2 maps to the host's
/// localhost; change this to your server's address for real devices.
const String kDefaultBaseUrl = 'http://10.0.2.2:8080';

/// Top-level app state: authentication, the API client and the device list.
class AppState extends ChangeNotifier {
  AppState._(this._prefs, this._baseUrl, String? token)
    : _api = ApiClient(baseUrl: _baseUrl, token: token);

  final SharedPreferences _prefs;
  String _baseUrl;
  final ApiClient _api;

  List<Device> _devices = const [];
  bool _loading = false;
  String? _error;

  static const _kToken = 'auth_token';
  static const _kBaseUrl = 'base_url';

  /// Loads persisted auth state from disk.
  static Future<AppState> load() async {
    final prefs = await SharedPreferences.getInstance();
    final baseUrl = prefs.getString(_kBaseUrl) ?? kDefaultBaseUrl;
    final token = prefs.getString(_kToken);
    return AppState._(prefs, baseUrl, token);
  }

  bool get isSignedIn => _api.token != null;
  List<Device> get devices => _devices;
  bool get loading => _loading;
  String? get error => _error;
  String get baseUrl => _baseUrl;

  Future<void> setBaseUrl(String url) async {
    _baseUrl = url.trim();
    await _prefs.setString(_kBaseUrl, _baseUrl);
    notifyListeners();
  }

  Future<void> signup(String email, String password) async {
    final token = await _api.signup(email, password);
    await _prefs.setString(_kToken, token);
    notifyListeners();
  }

  Future<void> login(String email, String password) async {
    final token = await _api.login(email, password);
    await _prefs.setString(_kToken, token);
    notifyListeners();
  }

  Future<void> signOut() async {
    _api.token = null;
    _devices = const [];
    await _prefs.remove(_kToken);
    notifyListeners();
  }

  /// Fetches devices, updating loading/error flags for the UI.
  Future<void> refreshDevices() async {
    _loading = true;
    _error = null;
    notifyListeners();
    try {
      _devices = await _api.listDevices();
    } on ApiException catch (e) {
      _error = e.message;
      if (e.statusCode == 401) {
        await signOut();
      }
    } catch (e) {
      _error = 'Could not reach the server.';
    } finally {
      _loading = false;
      notifyListeners();
    }
  }

  Future<void> claimDevice({String? pairingToken, String? code}) async {
    await _api.claimDevice(pairingToken: pairingToken, code: code);
    await refreshDevices();
  }
}
