import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:provider/provider.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:pc_status_app/src/models/device.dart';
import 'package:pc_status_app/src/models/device_lock.dart';
import 'package:pc_status_app/src/screens/lock_screen.dart';
import 'package:pc_status_app/src/state/app_state.dart';

void main() {
  test('lock status distinguishes requests from acknowledgements', () {
    final lock = DeviceLock.fromJson({
      'revision': 2,
      'desired_locked': true,
      'password_ready': true,
      'applied_revision': 1,
      'applied_locked': false,
      'last_error': '',
    });
    expect(lock.pending, true);
    expect(lock.summary, 'Lock requested - waiting for PC');
  });

  testWidgets('recovery setup, lock confirmation, unlock and pending status', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues({'auth_token': 'test-token'});
    var revision = 0;
    var passwordReady = false;
    var desiredLocked = false;
    final commands = <Map<String, dynamic>>[];
    final client = MockClient((request) async {
      if (request.method == 'POST') {
        expect(request.url.path, '/api/v1/devices/pc/lock');
        final body = jsonDecode(request.body) as Map<String, dynamic>;
        commands.add(body);
        revision++;
        desiredLocked = body['locked'] as bool;
        if (body.containsKey('password')) passwordReady = true;
        return http.Response(
          '{"status":"pending"}',
          202,
          headers: {'content-type': 'application/json'},
        );
      }
      return http.Response(
        jsonEncode({
          'devices': [
            {
              'id': 'pc',
              'name': 'Family PC',
              'status': 'ONLINE',
              'lock': {
                'revision': revision,
                'desired_locked': desiredLocked,
                'password_ready': passwordReady,
                'applied_revision': -1,
                'applied_locked': false,
                'last_error': '',
              },
            },
          ],
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    });
    final state = await AppState.load(client: client);
    await tester.pumpWidget(
      ChangeNotifierProvider.value(
        value: state,
        child: MaterialApp(
          home: LockScreen(
            device: Device(
              id: 'pc',
              name: 'Family PC',
              status: DeviceStatus.online,
            ),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.widgetWithText(FilledButton, 'Lock'), findsNothing);
    await tester.scrollUntilVisible(
      find.byKey(const Key('recovery-password')),
      200,
      scrollable: find.byType(Scrollable).first,
    );
    await tester.enterText(
      find.byKey(const Key('recovery-password')),
      'parent-password',
    );
    await tester.enterText(
      find.byKey(const Key('recovery-confirmation')),
      'parent-password',
    );
    await tester.ensureVisible(find.text('Save recovery password'));
    await tester.tap(find.text('Save recovery password'));
    await tester.pumpAndSettle();
    expect(commands.single['password'], 'parent-password');
    expect(commands.single['locked'], false);
    expect(
      tester
          .widget<TextFormField>(find.byType(TextFormField).first)
          .controller!
          .text,
      '',
    );
    await tester.scrollUntilVisible(
      find.text('Lock'),
      -200,
      scrollable: find.byType(Scrollable).first,
    );
    await tester.tap(find.text('Lock'));
    await tester.pumpAndSettle();
    expect(find.text('Lock this computer?'), findsOneWidget);
    await tester.tap(find.widgetWithText(FilledButton, 'Lock').last);
    await tester.pumpAndSettle();
    expect(commands.last['locked'], true);
    expect(commands.last.containsKey('password'), false);
    await tester.scrollUntilVisible(
      find.text('Lock requested - waiting for PC'),
      -200,
      scrollable: find.byType(Scrollable).first,
    );
    expect(find.text('Lock requested - waiting for PC'), findsOneWidget);
    await tester.scrollUntilVisible(
      find.text('Unlock'),
      200,
      scrollable: find.byType(Scrollable).first,
    );
    await tester.tap(find.text('Unlock'));
    await tester.pumpAndSettle();
    expect(commands.last['locked'], false);
    await tester.scrollUntilVisible(
      find.text('Unlock requested - waiting for PC'),
      -200,
      scrollable: find.byType(Scrollable).first,
    );
    expect(find.text('Unlock requested - waiting for PC'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
    state.dispose();
  });
}
