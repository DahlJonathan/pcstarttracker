import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:provider/provider.dart';

import '../models/device.dart';
import '../models/device_event.dart';
import '../state/app_state.dart';
import '../widgets/auto_refresh.dart';

/// Shows one device's boot/shutdown history, grouped by day.
class DeviceHistoryScreen extends StatefulWidget {
  const DeviceHistoryScreen({super.key, required this.device});

  final Device device;

  @override
  State<DeviceHistoryScreen> createState() => _DeviceHistoryScreenState();
}

class _DeviceHistoryScreenState extends State<DeviceHistoryScreen>
    with WidgetsBindingObserver, AutoRefresh<DeviceHistoryScreen> {
  late Future<List<DeviceEvent>> _future;
  bool _refreshingHistory = false;
  String? _refreshError;

  @override
  Future<void> refreshContent() => _refresh();

  @override
  void initState() {
    super.initState();
    _future = _load();
  }

  Future<List<DeviceEvent>> _load() =>
      context.read<AppState>().deviceHistory(widget.device.id);

  Future<void> _refresh() async {
    if (_refreshingHistory) return;
    _refreshingHistory = true;
    try {
      final events = await _load();
      if (mounted) {
        setState(() {
          _future = Future.value(events);
          _refreshError = null;
        });
      }
    } catch (error, stack) {
      debugPrint('History refresh failed: $error\n$stack');
      if (mounted) {
        setState(
          () => _refreshError = 'Could not update history. Pull down to retry.',
        );
      }
    } finally {
      _refreshingHistory = false;
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text(widget.device.name),
        actions: [
          IconButton(
            onPressed: _refresh,
            tooltip: 'Refresh history',
            icon: const Icon(Icons.refresh),
          ),
        ],
      ),
      body: Column(
        children: [
          if (_refreshError != null)
            Padding(
              padding: const EdgeInsets.all(12),
              child: Text(
                _refreshError!,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ),
          Expanded(
            child: RefreshIndicator(
              onRefresh: _refresh,
              child: FutureBuilder<List<DeviceEvent>>(
                future: _future,
                builder: (context, snapshot) {
                  if (snapshot.connectionState == ConnectionState.waiting) {
                    return const Center(child: CircularProgressIndicator());
                  }
                  if (snapshot.hasError) {
                    return _message(
                      'Could not load history. Pull down to retry.',
                    );
                  }
                  final events = snapshot.data ?? const [];
                  if (events.isEmpty) {
                    return _message(
                      'Nothing here yet.\nActivity will show up once the computer turns on or off.',
                    );
                  }
                  return _buildList(context, events);
                },
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _message(String text) => ListView(
    physics: const AlwaysScrollableScrollPhysics(),
    children: [
      SizedBox(height: MediaQuery.of(context).size.height * 0.3),
      Center(child: Text(text, textAlign: TextAlign.center)),
    ],
  );

  Widget _buildList(BuildContext context, List<DeviceEvent> events) {
    final groups = _groupByDay(events);
    final days = groups.keys.toList();

    return ListView.builder(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.only(bottom: 24),
      itemCount: days.length,
      itemBuilder: (_, i) {
        final day = days[i];
        final dayEvents = groups[day]!;
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 20, 16, 8),
              child: Text(
                _dayLabel(day),
                style: Theme.of(
                  context,
                ).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w700),
              ),
            ),
            ...dayEvents.map((e) => _EventTile(event: e)),
            const Divider(height: 1),
          ],
        );
      },
    );
  }

  /// Groups events into day buckets (newest day first), each sorted newest-first.
  Map<DateTime, List<DeviceEvent>> _groupByDay(List<DeviceEvent> events) {
    final map = <DateTime, List<DeviceEvent>>{};
    for (final e in events) {
      final day = DateTime(e.at.year, e.at.month, e.at.day);
      map.putIfAbsent(day, () => []).add(e);
    }
    return map;
  }

  String _dayLabel(DateTime day) {
    final now = DateTime.now();
    final today = DateTime(now.year, now.month, now.day);
    final yesterday = today.subtract(const Duration(days: 1));
    if (day == today) return 'Today';
    if (day == yesterday) return 'Yesterday';
    return DateFormat('EEEE, d MMM yyyy').format(day);
  }
}

class _EventTile extends StatelessWidget {
  const _EventTile({required this.event});

  final DeviceEvent event;

  @override
  Widget build(BuildContext context) {
    final boot = event.isBoot;
    final color = boot ? const Color(0xFF22C55E) : const Color(0xFF94A3B8);
    return ListTile(
      leading: Container(
        width: 42,
        height: 42,
        decoration: BoxDecoration(
          color: color.withValues(alpha: 0.14),
          borderRadius: BorderRadius.circular(12),
        ),
        child: Icon(
          boot ? Icons.power_rounded : Icons.power_settings_new_rounded,
          color: color,
        ),
      ),
      title: Text(
        boot ? 'Turned on' : 'Turned off',
        style: const TextStyle(fontWeight: FontWeight.w600),
      ),
      trailing: Text(
        event.time,
        style: Theme.of(context).textTheme.titleMedium,
      ),
    );
  }
}
