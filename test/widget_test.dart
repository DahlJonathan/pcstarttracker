// Basic smoke test for the PC Status app.

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:provider/provider.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:pc_status_app/src/screens/login_screen.dart';
import 'package:pc_status_app/src/state/app_state.dart';

void main() {
  testWidgets('shows login screen when signed out', (
    WidgetTester tester,
  ) async {
    SharedPreferences.setMockInitialValues({});
    final state = await AppState.load();

    await tester.pumpWidget(
      ChangeNotifierProvider.value(
        value: state,
        child: const MaterialApp(home: LoginScreen()),
      ),
    );

    expect(find.widgetWithText(FilledButton, 'Sign in'), findsOneWidget);
  });
}
