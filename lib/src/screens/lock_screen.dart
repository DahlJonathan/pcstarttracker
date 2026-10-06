import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_client.dart';
import '../models/device.dart';
import '../state/app_state.dart';
import '../widgets/auto_refresh.dart';
import '../widgets/screen_layout.dart';

class LockScreen extends StatefulWidget {
  const LockScreen({super.key, required this.device});
  final Device device;

  @override
  State<LockScreen> createState() => _LockScreenState();
}

class _LockScreenState extends State<LockScreen>
    with WidgetsBindingObserver, AutoRefresh<LockScreen> {
  final _password = TextEditingController();
  final _confirm = TextEditingController();
  final _form = GlobalKey<FormState>();
  bool _busy = false;
  String? _error;
  String? _notice;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) refreshContent();
    });
  }

  @override
  Future<void> refreshContent() async {
    final state = context.read<AppState>();
    if (state.isSignedIn) await state.refreshDevices();
  }

  @override
  void dispose() {
    _password.dispose();
    _confirm.dispose();
    super.dispose();
  }

  Future<void> _send(Device device, bool locked, {String? password}) async {
    setState(() {
      _busy = true;
      _error = null;
      _notice = null;
    });
    try {
      await context.read<AppState>().setDeviceLock(
        device.id,
        locked: locked,
        password: password,
      );
      if (!mounted) return;
      _password.clear();
      _confirm.clear();
      setState(
        () => _notice = 'Request saved. Waiting for the computer to confirm.',
      );
    } on ApiException catch (error) {
      if (mounted) setState(() => _error = error.message);
    } catch (error, stack) {
      debugPrint('Lock request failed: $error\n$stack');
      if (mounted) {
        setState(
          () => _error =
              'Could not confirm the request. Check your connection and refresh before retrying.',
        );
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = context.watch<AppState>();
    final current = state.devices
        .where((d) => d.id == widget.device.id)
        .firstOrNull;
    final device = current ?? widget.device;
    final lock = device.lock;
    final theme = Theme.of(context);
    return Scaffold(
      appBar: AppBar(title: const Text('Parental lock')),
      body: ScreenLayout(
          maxWidth: 560,
          child: ListView(
            padding: const EdgeInsets.all(20),
            children: [
              SectionHeader(
                title: device.name,
                subtitle: 'Manage access and recovery for this computer.',
                icon: lock?.appliedLocked == true
                    ? Icons.lock_rounded
                    : Icons.lock_open_rounded,
              ),
              const SizedBox(height: 24),
              Card(
                child: Padding(
                  padding: const EdgeInsets.all(20),
                  child: Text(
                lock?.summary ?? 'Update the server to enable parental locking',
                style: theme.textTheme.titleMedium?.copyWith(
                  color: theme.colorScheme.primary,
                ),
                  ),
                ),
              ),
              const SizedBox(height: 12),
              if (!device.isOnline)
                const Padding(
                  padding: EdgeInsets.only(top: 12),
                  child: Text(
                    'The PC is offline. A new request will wait for it to reconnect.',
                  ),
                ),
              if (state.error != null)
                Text(
                  'Could not refresh status: ${state.error}',
                  style: TextStyle(color: theme.colorScheme.error),
                ),
              if (lock?.lastError.isNotEmpty == true)
                Padding(
                  padding: const EdgeInsets.only(top: 12),
                  child: Text(
                    lock!.lastError,
                    style: TextStyle(color: theme.colorScheme.error),
                  ),
                ),
              if (_error != null)
                Padding(
                  padding: const EdgeInsets.only(top: 12),
                  child: Text(
                    _error!,
                    style: TextStyle(color: theme.colorScheme.error),
                  ),
                ),
              if (_notice != null && lock?.pending != false)
                Padding(
                  padding: const EdgeInsets.only(top: 12),
                  child: Text(_notice!),
                ),
              const SizedBox(height: 24),
              if (lock != null && lock.passwordReady)
                Row(
                  children: [
                    Expanded(
                      child: FilledButton.icon(
                        onPressed: _busy || current == null
                            ? null
                            : () async {
                                final approved = await showDialog<bool>(
                                  context: context,
                                  builder: (ctx) => AlertDialog(
                                    title: const Text('Lock this computer?'),
                                    content: const Text(
                                      'This will interrupt computer use when the request arrives. Make sure you remember the recovery password.',
                                    ),
                                    actions: [
                                      TextButton(
                                        onPressed: () =>
                                            Navigator.pop(ctx, false),
                                        child: const Text('Cancel'),
                                      ),
                                      FilledButton(
                                        onPressed: () =>
                                            Navigator.pop(ctx, true),
                                        child: const Text('Lock'),
                                      ),
                                    ],
                                  ),
                                );
                                if (approved == true && mounted) {
                                  await _send(device, true);
                                }
                              },
                        icon: const Icon(Icons.lock_outline),
                        label: const Text('Lock'),
                      ),
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                      child: OutlinedButton.icon(
                        onPressed: _busy || current == null
                            ? null
                            : () => _send(device, false),
                        icon: const Icon(Icons.lock_open),
                        label: const Text('Unlock'),
                      ),
                    ),
                  ],
                ),
              if (lock != null) ...[
                const SizedBox(height: 24),
                Text(
                  lock.passwordReady
                      ? 'Change recovery password'
                      : 'Set recovery password',
                  style: theme.textTheme.titleMedium,
                ),
                const SizedBox(height: 8),
                const Text(
                  'Use a separate password, not your Windows or phone password. '
                  'It is sent securely to the server; only a password hash is stored. '
                  'A change works offline only after the computer confirms it.',
                ),
                const SizedBox(height: 16),
                Form(
                  key: _form,
                  child: Column(
                    children: [
                      TextFormField(
                        key: const Key('recovery-password'),
                        controller: _password,
                        obscureText: true,
                        enableSuggestions: false,
                        autocorrect: false,
                        decoration: const InputDecoration(
                          labelText: 'Recovery password',
                        ),
                        validator: (value) {
                          final count = utf8.encode(value ?? '').length;
                          return count >= 8 && count <= 72
                              ? null
                              : 'Use 8 to 72 UTF-8 bytes (at least 8 ASCII characters)';
                        },
                      ),
                      const SizedBox(height: 12),
                      TextFormField(
                        key: const Key('recovery-confirmation'),
                        controller: _confirm,
                        obscureText: true,
                        enableSuggestions: false,
                        autocorrect: false,
                        decoration: const InputDecoration(
                          labelText: 'Repeat password',
                        ),
                        validator: (value) => value == _password.text
                            ? null
                            : 'Passwords do not match',
                      ),
                      const SizedBox(height: 16),
                      FilledButton(
                        onPressed: _busy || current == null
                            ? null
                            : () {
                                if (_form.currentState!.validate()) {
                                  _send(
                                    device,
                                    lock.desiredLocked,
                                    password: _password.text,
                                  );
                                }
                              },
                        child: const Text('Save recovery password'),
                      ),
                    ],
                  ),
                ),
              ],
              const SizedBox(height: 16),
              TextButton.icon(
                onPressed: _busy ? null : refreshContent,
                icon: const Icon(Icons.refresh),
                label: const Text('Refresh status'),
              ),
              if (_busy) const Center(child: CircularProgressIndicator()),
            ],
          ),
        ),
    );
  }
}
