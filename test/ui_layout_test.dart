import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:provider/provider.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:pc_status_app/main.dart';
import 'package:pc_status_app/src/models/device.dart';
import 'package:pc_status_app/src/screens/dashboard_screen.dart';
import 'package:pc_status_app/src/screens/device_history_screen.dart';
import 'package:pc_status_app/src/screens/login_screen.dart';
import 'package:pc_status_app/src/screens/lock_screen.dart';
import 'package:pc_status_app/src/screens/pair_screen.dart';
import 'package:pc_status_app/src/state/app_state.dart';
import 'package:pc_status_app/src/widgets/device_card.dart';
import 'package:pc_status_app/src/widgets/screen_layout.dart';

Widget app(AppState state, Widget home, Brightness brightness) {
  return ChangeNotifierProvider.value(
    value: state,
    child: MaterialApp(theme: buildAppTheme(brightness), home: home),
  );
}

void configureViewport(WidgetTester tester, Size size) {
  tester.view.devicePixelRatio = 1;
  tester.view.physicalSize = size;
  tester.platformDispatcher.textScaleFactorTestValue = 2;
  addTearDown(tester.view.reset);
  addTearDown(tester.platformDispatcher.clearTextScaleFactorTestValue);
}

void main() {
  for (final brightness in Brightness.values) {
    for (final size in [const Size(320, 640), const Size(640, 320)]) {
      testWidgets(
        'login fits $size in $brightness with large text and keyboard',
        (tester) async {
          configureViewport(tester, size);
          SharedPreferences.setMockInitialValues({});
          final state = await AppState.load();
          await tester.pumpWidget(app(state, const LoginScreen(), brightness));
          await tester.pumpAndSettle();
          expect(tester.takeException(), isNull);

          tester.view.viewInsets = FakeViewPadding(bottom: size.height * 0.4);
          await tester.pumpAndSettle();
          await tester.ensureVisible(
            find.widgetWithText(FilledButton, 'Sign in'),
          );
          await tester.pumpAndSettle();
          expect(tester.takeException(), isNull);
          await tester.tap(find.widgetWithText(FilledButton, 'Sign in'));
          await tester.pumpAndSettle();
          expect(find.text('Enter a valid email'), findsOneWidget);
          expect(find.text('At least 8 characters'), findsOneWidget);
          expect(tester.takeException(), isNull);
          await tester.pumpWidget(const SizedBox());
          state.dispose();
        },
      );
    }

    testWidgets(
      'device card fits long names and keeps actions in $brightness',
      (tester) async {
        configureViewport(tester, const Size(320, 640));
        var activityTaps = 0;
        var lockTaps = 0;
        var removals = 0;
        await tester.pumpWidget(
          MaterialApp(
            theme: buildAppTheme(brightness),
            home: Scaffold(
              body: SingleChildScrollView(
                padding: const EdgeInsets.all(20),
                child: DeviceCard(
                  device: Device(
                    id: 'pc',
                    name: 'The family computer with a very long device name',
                    status: DeviceStatus.online,
                    lastBootAt: DateTime(2026, 10, 6, 8),
                  ),
                  onTap: () => activityTaps++,
                  onLock: () => lockTaps++,
                  onDelete: () => removals++,
                ),
              ),
            ),
          ),
        );
        await tester.pumpAndSettle();
        expect(tester.takeException(), isNull);
        await tester.ensureVisible(find.text('Activity'));
        await tester.tap(find.text('Activity'));
        expect(activityTaps, 1);
        await tester.ensureVisible(find.text('Parental lock'));
        await tester.tap(find.text('Parental lock'));
        expect(lockTaps, 1);
        await tester.ensureVisible(find.byTooltip('Options'));
        await tester.tap(find.byTooltip('Options'));
        await tester.pumpAndSettle();
        await tester.tap(find.text('Remove'));
        await tester.pumpAndSettle();
        expect(removals, 1);
        expect(tester.takeException(), isNull);
      },
    );

    testWidgets('dashboard summary reflects devices in $brightness', (
      tester,
    ) async {
      configureViewport(tester, const Size(320, 640));
      SharedPreferences.setMockInitialValues({'auth_token': 'test-token'});
      final state = await AppState.load(
        client: MockClient(
          (_) async => http.Response(
            jsonEncode({
              'devices': [
                {'id': '1', 'name': 'Office PC', 'status': 'ONLINE'},
                {'id': '2', 'name': 'Living room PC', 'status': 'OFFLINE'},
                {'id': '3', 'name': 'Family PC', 'status': 'ONLINE'},
              ],
            }),
            200,
            headers: {'content-type': 'application/json'},
          ),
        ),
      );
      await tester.pumpWidget(app(state, const DashboardScreen(), brightness));
      await tester.pumpAndSettle();
      final panel = find.byType(BrandPanel);
      expect(
        find.descendant(of: panel, matching: find.text('3')),
        findsOneWidget,
      );
      expect(
        find.descendant(of: panel, matching: find.text('2')),
        findsOneWidget,
      );
      expect(
        find.descendant(of: panel, matching: find.text('1')),
        findsOneWidget,
      );
      await tester.scrollUntilVisible(find.text('Office PC'), 200);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
      state.dispose();
    });

    testWidgets('pairing code stays reachable with keyboard in $brightness', (
      tester,
    ) async {
      configureViewport(tester, const Size(320, 640));
      SharedPreferences.setMockInitialValues({});
      final state = await AppState.load();
      await tester.pumpWidget(app(state, const PairScreen(), brightness));
      await tester.pumpAndSettle();
      tester.view.viewInsets = const FakeViewPadding(bottom: 280);
      await tester.pumpAndSettle();
      await tester.scrollUntilVisible(find.byType(TextField), 200);
      await tester.enterText(find.byType(TextField), '123');
      await tester.ensureVisible(find.widgetWithText(FilledButton, 'Add'));
      await tester.tap(find.widgetWithText(FilledButton, 'Add'));
      await tester.pumpAndSettle();
      expect(find.text('Enter the 6-digit code'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
      state.dispose();
    });

    for (final history in [true, false]) {
      testWidgets('secondary screen history=$history fits in $brightness', (
        tester,
      ) async {
        configureViewport(tester, const Size(320, 640));
        SharedPreferences.setMockInitialValues({'auth_token': 'test-token'});
        final state = await AppState.load(
          client: MockClient(
            (_) async => http.Response(
              jsonEncode(
                history
                    ? {
                        'events': [
                          {
                            'event': 'boot',
                            'created_at': '2026-10-06T08:00:00Z',
                          },
                        ],
                      }
                    : {
                        'devices': [
                          {
                            'id': 'pc',
                            'name': 'Family computer with a long name',
                            'status': 'ONLINE',
                            'lock': {
                              'revision': 2,
                              'desired_locked': true,
                              'password_ready': true,
                              'applied_revision': 1,
                              'applied_locked': false,
                              'last_error': '',
                            },
                          },
                        ],
                      },
              ),
              200,
              headers: {'content-type': 'application/json'},
            ),
          ),
        );
        final device = Device(
          id: 'pc',
          name: 'Family computer with a long name',
          status: DeviceStatus.online,
        );
        await tester.pumpWidget(
          app(
            state,
            history
                ? DeviceHistoryScreen(device: device)
                : LockScreen(device: device),
            brightness,
          ),
        );
        await tester.pumpAndSettle();
        await tester.scrollUntilVisible(
          find.text(history ? 'Turned on' : 'Unlock'),
          200,
        );
        expect(tester.takeException(), isNull);
        await tester.pumpWidget(const SizedBox());
        state.dispose();
      });
    }

    testWidgets('status labels have readable contrast in $brightness', (
      tester,
    ) async {
      final theme = buildAppTheme(brightness);
      for (final status in DeviceStatus.values) {
        await tester.pumpWidget(
          MaterialApp(
            theme: theme,
            home: Scaffold(
              body: DeviceCard(
                device: Device(id: 'pc', name: 'PC', status: status),
              ),
            ),
          ),
        );
        final label = tester.widget<Text>(
          find.text(status == DeviceStatus.online ? 'On' : 'Offline'),
        );
        final foreground = label.style!.color!;
        final background = Color.alphaBlend(
          foreground.withValues(alpha: 0.12),
          theme.cardTheme.color!,
        );
        final foregroundLuminance = foreground.computeLuminance();
        final backgroundLuminance = background.computeLuminance();
        final ratio = (backgroundLuminance > foregroundLuminance)
            ? (backgroundLuminance + 0.05) / (foregroundLuminance + 0.05)
            : (foregroundLuminance + 0.05) / (backgroundLuminance + 0.05);
        expect(ratio, greaterThanOrEqualTo(4.5));
      }
    });
  }
}
