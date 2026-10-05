import 'dart:async';

import 'package:flutter/material.dart';
import 'package:mobile_scanner/mobile_scanner.dart';
import 'package:provider/provider.dart';

import '../api/api_client.dart';
import '../state/app_state.dart';

/// Pairing screen: scan the computer's QR code or enter the 6-digit code.
///
/// The camera lifecycle is managed here because a user-provided
/// [MobileScannerController] opts out of the plugin's automatic start/stop.
/// Without this, the camera fails to (re)start after a permission prompt or
/// when the app returns to the foreground, surfacing as "camera could not be
/// started".
class PairScreen extends StatefulWidget {
  const PairScreen({super.key});

  @override
  State<PairScreen> createState() => _PairScreenState();
}

class _PairScreenState extends State<PairScreen> with WidgetsBindingObserver {
  final _scanner = MobileScannerController(autoStart: false);
  final _code = TextEditingController();
  bool _busy = false;
  bool _handled = false; // prevents duplicate claims from rapid scans

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _startCamera();
  }

  Future<void> _startCamera() async {
    try {
      await _scanner.start();
    } catch (_) {
      // Any failure is surfaced through MobileScanner's errorBuilder.
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (!_scanner.value.isInitialized) return;
    switch (state) {
      case AppLifecycleState.resumed:
        _startCamera();
      case AppLifecycleState.inactive:
      case AppLifecycleState.paused:
      case AppLifecycleState.hidden:
      case AppLifecycleState.detached:
        unawaited(_scanner.stop());
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _scanner.dispose();
    _code.dispose();
    super.dispose();
  }

  Future<void> _claim({String? pairingToken, String? code}) async {
    if (_busy) return;
    setState(() => _busy = true);
    try {
      await context.read<AppState>().claimDevice(
        pairingToken: pairingToken,
        code: code,
      );
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Computer added 🎉')),
      );
      Navigator.of(context).pop(true);
    } on ApiException catch (e) {
      _showError(e.message);
    } catch (_) {
      _showError('Could not reach the server.');
    } finally {
      if (mounted) setState(() => _busy = false);
      _handled = false;
    }
  }

  void _showError(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  void _onDetect(BarcodeCapture capture) {
    if (_handled || _busy) return;
    final raw = capture.barcodes.firstOrNull?.rawValue;
    if (raw == null || raw.isEmpty) return;
    _handled = true;
    _claim(pairingToken: raw);
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Scaffold(
      appBar: AppBar(title: const Text('Add a computer')),
      body: Column(
        children: [
          Expanded(
            child: Container(
              margin: const EdgeInsets.fromLTRB(16, 8, 16, 16),
              clipBehavior: Clip.antiAlias,
              decoration: BoxDecoration(
                color: Colors.black,
                borderRadius: BorderRadius.circular(24),
              ),
              child: Stack(
                alignment: Alignment.center,
                children: [
                  MobileScanner(
                    controller: _scanner,
                    onDetect: _onDetect,
                    errorBuilder: (context, error, child) =>
                        _CameraError(error: error, onRetry: _startCamera),
                  ),
                  const _ScannerOverlay(),
                  Positioned(
                    bottom: 16,
                    child: Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 16,
                        vertical: 8,
                      ),
                      decoration: BoxDecoration(
                        color: Colors.black54,
                        borderRadius: BorderRadius.circular(999),
                      ),
                      child: const Text(
                        'Point the camera at the QR code',
                        style: TextStyle(color: Colors.white),
                      ),
                    ),
                  ),
                  if (_busy)
                    Container(
                      color: Colors.black45,
                      child: const Center(child: CircularProgressIndicator()),
                    ),
                ],
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 0, 20, 24),
            child: Column(
              children: [
                Row(
                  children: [
                    const Expanded(child: Divider()),
                    Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 12),
                      child: Text(
                        'or type the code',
                        style: theme.textTheme.bodySmall?.copyWith(
                          color: theme.colorScheme.outline,
                        ),
                      ),
                    ),
                    const Expanded(child: Divider()),
                  ],
                ),
                const SizedBox(height: 16),
                Row(
                  children: [
                    Expanded(
                      child: TextField(
                        controller: _code,
                        keyboardType: TextInputType.number,
                        maxLength: 6,
                        style: const TextStyle(
                          fontSize: 22,
                          letterSpacing: 6,
                          fontWeight: FontWeight.w600,
                        ),
                        decoration: const InputDecoration(
                          labelText: '6-digit code',
                          counterText: '',
                        ),
                      ),
                    ),
                    const SizedBox(width: 12),
                    FilledButton(
                      onPressed: _busy
                          ? null
                          : () {
                              final c = _code.text.trim();
                              if (c.length == 6) {
                                _claim(code: c);
                              } else {
                                _showError('Enter the 6-digit code');
                              }
                            },
                      child: const Text('Add'),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _ScannerOverlay extends StatelessWidget {
  const _ScannerOverlay();

  @override
  Widget build(BuildContext context) {
    return IgnorePointer(
      child: Container(
        width: 240,
        height: 240,
        decoration: BoxDecoration(
          border: Border.all(color: Colors.white, width: 3),
          borderRadius: BorderRadius.circular(24),
        ),
      ),
    );
  }
}

/// Shown when the camera cannot start (most often a denied permission).
class _CameraError extends StatelessWidget {
  const _CameraError({required this.error, required this.onRetry});

  final MobileScannerException error;
  final Future<void> Function() onRetry;

  @override
  Widget build(BuildContext context) {
    final denied = error.errorCode == MobileScannerErrorCode.permissionDenied;
    return Container(
      color: Colors.black,
      padding: const EdgeInsets.all(28),
      alignment: Alignment.center,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Icon(
            Icons.photo_camera_outlined,
            size: 56,
            color: Colors.white70,
          ),
          const SizedBox(height: 16),
          Text(
            denied
                ? 'We need camera access to scan the QR code.\n\n'
                      'Turn on Camera for this app in your phone settings, '
                      'then tap Try again — or just type the 6-digit code below.'
                : 'The camera could not be started.\n\n'
                      'No problem — type the 6-digit code shown on your '
                      'computer below instead.',
            textAlign: TextAlign.center,
            style: const TextStyle(color: Colors.white, height: 1.4),
          ),
          const SizedBox(height: 20),
          FilledButton.icon(
            onPressed: onRetry,
            icon: const Icon(Icons.refresh),
            label: const Text('Try again'),
          ),
        ],
      ),
    );
  }
}
