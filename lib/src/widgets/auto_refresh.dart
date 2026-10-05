import 'dart:async';

import 'package:flutter/material.dart';

/// Refresh only the visible route while the app is in the foreground.
mixin AutoRefresh<T extends StatefulWidget>
    on State<T>, WidgetsBindingObserver {
  Timer? _refreshTimer;
  bool _refreshing = false;
  bool _foreground = true;

  Future<void> refreshContent();

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _refreshTimer = Timer.periodic(
      const Duration(seconds: 10),
      (_) => _refreshVisible(),
    );
  }

  Future<void> _refreshVisible() async {
    if (!mounted ||
        !_foreground ||
        _refreshing ||
        ModalRoute.of(context)?.isCurrent == false) {
      return;
    }
    _refreshing = true;
    try {
      await refreshContent();
    } finally {
      _refreshing = false;
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    _foreground = state == AppLifecycleState.resumed;
    if (_foreground) unawaited(_refreshVisible());
  }

  @override
  void dispose() {
    _refreshTimer?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }
}
