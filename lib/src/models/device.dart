import 'package:intl/intl.dart';
import 'device_lock.dart';

/// Computed power state of a device, mirroring the backend.
enum DeviceStatus { online, offline }

/// A paired PC as returned by `GET /api/v1/devices`.
class Device {
  Device({
    required this.id,
    required this.name,
    required this.status,
    this.lastEvent,
    this.lastBootAt,
    this.lastShutdownAt,
    this.lastHeartbeatAt,
    this.lock,
  });

  final String id;
  final String name;
  final DeviceStatus status;
  final String? lastEvent;
  final DateTime? lastBootAt;
  final DateTime? lastShutdownAt;
  final DateTime? lastHeartbeatAt;
  final DeviceLock? lock;

  bool get isOnline => status == DeviceStatus.online;

  factory Device.fromJson(Map<String, dynamic> json) {
    return Device(
      id: json['id'] as String,
      name: (json['name'] as String?) ?? 'My PC',
      status: (json['status'] as String?)?.toUpperCase() == 'ONLINE'
          ? DeviceStatus.online
          : DeviceStatus.offline,
      lastEvent: json['last_event'] as String?,
      lastBootAt: _parse(json['last_boot_at']),
      lastShutdownAt: _parse(json['last_shutdown_at']),
      lastHeartbeatAt: _parse(json['last_heartbeat_at']),
      lock: json['lock'] is Map<String, dynamic>
          ? DeviceLock.fromJson(json['lock'] as Map<String, dynamic>)
          : null,
    );
  }

  static DateTime? _parse(dynamic v) {
    if (v is String && v.isNotEmpty) {
      return DateTime.tryParse(v)?.toLocal();
    }
    return null;
  }

  /// A human-friendly summary line for the card, e.g.
  /// "Booted today at 08:15" or "Shut down 2 hours ago".
  String get statusDetail {
    if (isOnline) {
      if (lastBootAt != null) {
        return 'Turned on ${_relativeOrTime(lastBootAt!)}';
      }
      return 'On now';
    }
    if (lastEvent == 'shutdown' && lastShutdownAt != null) {
      return 'Turned off ${_ago(lastShutdownAt!)}';
    }
    if (lastHeartbeatAt != null) {
      return 'Connection lost. Last seen ${_ago(lastHeartbeatAt!)}';
    }
    return 'Off';
  }

  String get exactTimestamp {
    final t = isOnline
        ? lastBootAt
        : (lastEvent == 'shutdown' ? lastShutdownAt : lastHeartbeatAt);
    if (t == null) return '';
    return DateFormat('EEE d MMM, HH:mm').format(t);
  }
}

String _relativeOrTime(DateTime t) {
  final now = DateTime.now();
  final sameDay =
      t.year == now.year && t.month == now.month && t.day == now.day;
  final time = DateFormat('HH:mm').format(t);
  if (sameDay) return 'today at $time';
  final yesterday = now.subtract(const Duration(days: 1));
  if (t.year == yesterday.year &&
      t.month == yesterday.month &&
      t.day == yesterday.day) {
    return 'yesterday at $time';
  }
  return 'on ${DateFormat('d MMM').format(t)} at $time';
}

String _ago(DateTime t) {
  final d = DateTime.now().difference(t);
  if (d.inSeconds < 60) return 'just now';
  if (d.inMinutes < 60) {
    final m = d.inMinutes;
    return '$m minute${m == 1 ? '' : 's'} ago';
  }
  if (d.inHours < 24) {
    final h = d.inHours;
    return '$h hour${h == 1 ? '' : 's'} ago';
  }
  final days = d.inDays;
  return '$days day${days == 1 ? '' : 's'} ago';
}
