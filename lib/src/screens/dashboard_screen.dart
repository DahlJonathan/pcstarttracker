import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../state/app_state.dart';
import '../widgets/auto_refresh.dart';
import '../widgets/device_card.dart';
import '../widgets/screen_layout.dart';
import 'device_history_screen.dart';
import 'pair_screen.dart';
import 'lock_screen.dart';

/// Main dashboard listing the user's paired PCs with pull-to-refresh.
class DashboardScreen extends StatefulWidget {
  const DashboardScreen({super.key});

  @override
  State<DashboardScreen> createState() => _DashboardScreenState();
}

class _DashboardScreenState extends State<DashboardScreen>
    with WidgetsBindingObserver, AutoRefresh<DashboardScreen> {
  @override
  Future<void> refreshContent() async {
    final state = context.read<AppState>();
    if (state.isSignedIn) await state.refreshDevices();
  }

  @override
  void initState() {
    super.initState();
    // Fetch on open.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      context.read<AppState>().refreshDevices();
    });
  }

  Future<void> _openPairing() async {
    final added = await Navigator.of(
      context,
    ).push<bool>(MaterialPageRoute(builder: (_) => const PairScreen()));
    if (added == true && mounted) {
      await context.read<AppState>().refreshDevices();
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = context.watch<AppState>();

    return Scaffold(
      appBar: AppBar(
        title: const Text('My computers'),
        actions: [
          IconButton(
            icon: const Icon(Icons.logout_rounded),
            tooltip: 'Sign out',
            onPressed: () => context.read<AppState>().signOut(),
          ),
        ],
      ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: _openPairing,
        icon: const Icon(Icons.add_rounded),
        label: const Text('Add computer'),
      ),
      body: ScreenLayout(
        maxWidth: 760,
        child: RefreshIndicator(
        onRefresh: () => context.read<AppState>().refreshDevices(),
        child: Column(
          children: [
            if (state.error != null && state.devices.isNotEmpty)
              Padding(
                padding: const EdgeInsets.all(12),
                child: Text(
                  'Could not update: ${state.error}',
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                  ),
                ),
            Expanded(child: _buildBody(context, state)),
          ],
        ),
      ),
      ),
    );
  }

  Widget _buildBody(BuildContext context, AppState state) {
    final theme = Theme.of(context);
    if (state.loading && state.devices.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }
    if (state.devices.isEmpty) {
      // ListView keeps pull-to-refresh working even when empty.
      return ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: const EdgeInsets.fromLTRB(24, 32, 24, 120),
        children: [
          _Overview(state: state),
          const SizedBox(height: 48),
          Center(
            child: Container(
              width: 120,
              height: 120,
              decoration: BoxDecoration(
                color: theme.colorScheme.primaryContainer,
                borderRadius: BorderRadius.circular(32),
              ),
              child: Icon(
                Icons.devices_other_rounded,
                size: 64,
                color: theme.colorScheme.onPrimaryContainer,
              ),
            ),
          ),
          const SizedBox(height: 24),
          Center(
            child: Text(
              state.error ?? 'No computers yet',
              textAlign: TextAlign.center,
              style: theme.textTheme.titleLarge?.copyWith(
                fontWeight: FontWeight.w700,
              ),
            ),
          ),
          const SizedBox(height: 8),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 40),
            child: Text(
              state.error != null
                  ? 'Pull down to try again.'
                  : 'Tap “Add computer” below and scan the QR code shown on the computer you want to watch.',
              textAlign: TextAlign.center,
              style: theme.textTheme.bodyLarge?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
          ),
        ],
      );
    }
    return ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.fromLTRB(20, 12, 20, 120),
      children: [
        _Overview(state: state),
        const SizedBox(height: 28),
        Text('Your devices', style: theme.textTheme.titleLarge),
        const SizedBox(height: 6),
        Text(
          'Select a computer to view its activity or manage access.',
          style: theme.textTheme.bodyMedium?.copyWith(
            color: theme.colorScheme.onSurfaceVariant,
          ),
        ),
        const SizedBox(height: 18),
        for (final device in state.devices)
          DeviceCard(
          device: device,
          onDelete: () => _confirmRemove(device.id, device.name),
          onLock: () async {
            await Navigator.of(context).push(
              MaterialPageRoute(builder: (_) => LockScreen(device: device)),
            );
            if (mounted) await refreshContent();
          },
          onTap: () async {
            await Navigator.of(context).push(
              MaterialPageRoute(
                builder: (_) => DeviceHistoryScreen(device: device),
              ),
            );
            if (mounted) await refreshContent();
          },
          ),
      ],
    );
  }

  Future<void> _confirmRemove(String id, String name) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Remove this computer?'),
        content: Text(
          '“$name” will stop showing here. '
          'You can always add it again later.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('Remove'),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    try {
      await context.read<AppState>().removeDevice(id);
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('Removed "$name"')));
      }
    } catch (_) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Could not remove device')),
        );
      }
    }
  }
}

class _Overview extends StatelessWidget {
    const _Overview({required this.state});

    final AppState state;

    @override
    Widget build(BuildContext context) {
      final online = state.devices.where((device) => device.isOnline).length;
      final textTheme = Theme.of(context).textTheme;
      return BrandPanel(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Icon(Icons.desktop_windows_rounded, color: Color(0xFF8DE4D5)),
            const SizedBox(height: 20),
            Text(
              'Home PCs',
              style: textTheme.headlineMedium?.copyWith(color: Colors.white),
            ),
            const SizedBox(height: 8),
            Text(
              'Your computers, at a glance.',
              style: textTheme.bodyMedium?.copyWith(color: const Color(0xFFD5E3EC)),
            ),
            const SizedBox(height: 24),
            Wrap(
              spacing: 28,
              runSpacing: 16,
              children: [
                _OverviewMetric(label: 'Computers', value: '${state.devices.length}'),
                _OverviewMetric(label: 'Online now', value: '$online'),
                _OverviewMetric(
                  label: 'Offline now',
                  value: '${state.devices.length - online}',
                ),
              ],
            ),
            if (state.error != null || state.loading) ...[
              const SizedBox(height: 16),
              Text(
                state.error != null
                    ? 'Update failed. Showing last known status.'
                    : 'Updating status...',
                style: textTheme.bodySmall?.copyWith(color: const Color(0xFFD5E3EC)),
              ),
            ],
          ],
        ),
      );
    }
  }

  class _OverviewMetric extends StatelessWidget {
    const _OverviewMetric({required this.label, required this.value});

    final String label;
    final String value;

    @override
    Widget build(BuildContext context) {
      final textTheme = Theme.of(context).textTheme;
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            value,
            style: textTheme.headlineMedium?.copyWith(color: const Color(0xFF8DE4D5)),
          ),
          Text(
            label,
            style: textTheme.bodySmall?.copyWith(color: const Color(0xFFD5E3EC)),
          ),
        ],
      );
    }
  }
