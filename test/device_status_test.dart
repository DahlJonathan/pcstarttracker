import 'package:flutter_test/flutter_test.dart';
import 'package:intl/intl.dart';
import 'package:pc_status_app/src/models/device.dart';

void main() {
  test(
    'heartbeat loss does not claim a physical shutdown or use an old shutdown time',
    () {
      final lastSeen = DateTime.now().subtract(const Duration(minutes: 1));
      final oldShutdown = lastSeen.subtract(const Duration(days: 2));
      final device = Device(
        id: 'pc',
        name: 'PC',
        status: DeviceStatus.offline,
        lastEvent: 'boot',
        lastHeartbeatAt: lastSeen,
        lastShutdownAt: oldShutdown,
      );
      expect(device.statusDetail, startsWith('Connection lost. Last seen'));
      expect(
        device.exactTimestamp,
        DateFormat('EEE d MMM, HH:mm').format(lastSeen),
      );
    },
  );

  test('confirmed shutdown retains turned-off wording', () {
    final device = Device(
      id: 'pc',
      name: 'PC',
      status: DeviceStatus.offline,
      lastEvent: 'shutdown',
      lastShutdownAt: DateTime.now(),
    );
    expect(device.statusDetail, startsWith('Turned off'));
  });
}
