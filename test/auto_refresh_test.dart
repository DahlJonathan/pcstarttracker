import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:provider/provider.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:pc_status_app/src/models/device.dart';
import 'package:pc_status_app/src/screens/dashboard_screen.dart';
import 'package:pc_status_app/src/screens/device_history_screen.dart';
import 'package:pc_status_app/src/state/app_state.dart';

Future<AppState> makeState(MockClient client) {
  SharedPreferences.setMockInitialValues({'auth_token': 'test-token'});
  return AppState.load(client: client);
}

Widget wrap(AppState state, Widget screen) => ChangeNotifierProvider.value(
  value: state,
  child: MaterialApp(home: screen),
);

http.Response jsonResponse(Object value) => http.Response(
  jsonEncode(value),
  200,
  headers: {'content-type': 'application/json'},
);

void main() {
  testWidgets('dashboard polls and resumes, but pauses in the background', (
    tester,
  ) async {
    var requests = 0;
    final state = await makeState(
      MockClient((request) async {
        requests++;
        return jsonResponse({
          'devices': [
            {
              'id': 'pc',
              'name': 'Family PC',
              'status': requests == 1 ? 'OFFLINE' : 'ONLINE',
            },
          ],
        });
      }),
    );
    await tester.pumpWidget(wrap(state, const DashboardScreen()));
    await tester.pumpAndSettle();
    expect(find.text('Offline'), findsOneWidget);
    await tester.pump(const Duration(seconds: 10));
    await tester.pumpAndSettle();
    expect(find.text('On'), findsOneWidget);
    expect(requests, 2);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    await tester.pump(const Duration(seconds: 30));
    expect(requests, 2);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pumpAndSettle();
    expect(requests, 3);
    final dashboardContext = tester.element(find.byType(DashboardScreen));
    Navigator.of(dashboardContext).push(
      MaterialPageRoute<void>(
        builder: (_) => const Scaffold(body: Text('Another screen')),
      ),
    );
    await tester.pumpAndSettle();
    await tester.pump(const Duration(seconds: 20));
    expect(requests, 3);
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 20));
    expect(requests, 3);
    state.dispose();
  });

  testWidgets(
    'history automatically adds new events and reports refresh errors',
    (tester) async {
      var requests = 0;
      final state = await makeState(
        MockClient((request) async {
          requests++;
          if (requests == 3) {
            return http.Response(
              '{"error":"offline"}',
              503,
              headers: {'content-type': 'application/json'},
            );
          }
          return jsonResponse({
            'events': [
              if (requests >= 2)
                {'event': 'shutdown', 'created_at': '2026-10-05T05:31:00Z'},
              {'event': 'boot', 'created_at': '2026-10-05T05:30:00Z'},
            ],
          });
        }),
      );
      final pc = Device(
        id: 'pc',
        name: 'Family PC',
        status: DeviceStatus.online,
      );
      await tester.pumpWidget(wrap(state, DeviceHistoryScreen(device: pc)));
      await tester.pumpAndSettle();
      expect(find.text('Turned on'), findsOneWidget);
      expect(find.text('Turned off'), findsNothing);
      await tester.pump(const Duration(seconds: 10));
      await tester.pumpAndSettle();
      expect(find.text('Turned off'), findsOneWidget);
      await tester.pump(const Duration(seconds: 10));
      await tester.pumpAndSettle();
      expect(
        find.text('Could not update history. Pull down to retry.'),
        findsOneWidget,
      );
      expect(find.text('Turned off'), findsOneWidget);
      await tester.pumpWidget(const SizedBox());
      state.dispose();
    },
  );
}
