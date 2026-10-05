class DeviceLock {
  const DeviceLock({
    required this.revision,
    required this.desiredLocked,
    required this.passwordReady,
    required this.appliedRevision,
    required this.appliedLocked,
    required this.lastError,
  });

  final int revision;
  final bool desiredLocked;
  final bool passwordReady;
  final int appliedRevision;
  final bool appliedLocked;
  final String lastError;

  bool get pending =>
      revision != appliedRevision ||
      desiredLocked != appliedLocked ||
      lastError.isNotEmpty;

  String get summary {
    if (!passwordReady) return 'Set up parental lock';
    if (pending) {
      return desiredLocked
          ? 'Lock requested - waiting for PC'
          : 'Unlock requested - waiting for PC';
    }
    return appliedLocked ? 'Parental lock active' : 'Parental lock off';
  }

  factory DeviceLock.fromJson(Map<String, dynamic> json) => DeviceLock(
    revision: json['revision'] as int,
    desiredLocked: json['desired_locked'] as bool,
    passwordReady: json['password_ready'] as bool,
    appliedRevision: json['applied_revision'] as int,
    appliedLocked: json['applied_locked'] as bool,
    lastError: json['last_error'] as String? ?? '',
  );
}
