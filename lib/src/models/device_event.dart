import 'package:intl/intl.dart';

/// One boot or shutdown entry in a device's history.
class DeviceEvent {
  DeviceEvent({required this.event, required this.at});

  final String event; // "boot" | "shutdown"
  final DateTime at;

  bool get isBoot => event == 'boot';

  factory DeviceEvent.fromJson(Map<String, dynamic> json) {
    return DeviceEvent(
      event: (json['event'] as String?) ?? '',
      at:
          DateTime.tryParse((json['created_at'] as String?) ?? '')?.toLocal() ??
          DateTime.fromMillisecondsSinceEpoch(0),
    );
  }

  String get time => DateFormat('HH:mm').format(at);
}
