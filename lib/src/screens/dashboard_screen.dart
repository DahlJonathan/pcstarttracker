import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../state/app_state.dart';
import '../widgets/device_card.dart';
import 'device_history_screen.dart';
import 'pair_screen.dart';

/// Main dashboard listing the user's paired PCs with pull-to-refresh.
class DashboardScreen extends StatefulWidget {
  const DashboardScreen({super.key});

  @override
  State<DashboardScreen> createState() => _DashboardScreenState();
}

class _DashboardScreenState extends State<DashboardScreen> {
  @override
  void initState() {
    super.initState();
    // Fetch on open.
    WidgetsBinding.instance.addPostFrameCallback((_) {
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
        title: const Text('My PCs'),
        actions: [
          IconButton(
            icon: const Icon(Icons.logout),
            tooltip: 'Sign out',
            onPressed: () => context.read<AppState>().signOut(),
          ),
        ],
      ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: _openPairing,
        icon: const Icon(Icons.add),
        label: const Text('Add Device'),
      ),
      body: RefreshIndicator(
        onRefresh: () => context.read<AppState>().refreshDevices(),
        child: _buildBody(context, state),
      ),
    );
  }

  Widget _buildBody(BuildContext context, AppState state) {
    if (state.loading && state.devices.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }
    if (state.devices.isEmpty) {
      // ListView keeps pull-to-refresh working even when empty.
      return ListView(
        children: [
          SizedBox(height: MediaQuery.of(context).size.height * 0.25),
          Icon(
            Icons.devices_other_outlined,
            size: 72,
            color: Theme.of(context).colorScheme.outline,
          ),
          const SizedBox(height: 16),
          Center(
            child: Text(
              state.error ??
                  'No PCs yet.\nTap "Add Device" to pair your computer.',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyLarge,
            ),
          ),
        ],
      );
    }
    return ListView.builder(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.only(top: 8, bottom: 96),
      itemCount: state.devices.length,
      itemBuilder: (_, i) {
        final device = state.devices[i];
        return DeviceCard(
          device: device,
          onDelete: () => _confirmRemove(device.id, device.name),
          onTap: () => Navigator.of(context).push(
            MaterialPageRoute(
              builder: (_) => DeviceHistoryScreen(device: device),
            ),
          ),
        );
      },
    );
  }

  Future<void> _confirmRemove(String id, String name) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Remove device'),
        content: Text(
          'Remove "$name"? It will stop appearing here. '
          'You can pair the PC again later.',
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
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Removed "$name"')),
        );
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
