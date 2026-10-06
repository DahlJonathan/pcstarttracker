import 'package:flutter/material.dart';

class ScreenLayout extends StatelessWidget {
  const ScreenLayout({
    super.key,
    required this.child,
    this.maxWidth = 640,
  });

  final Widget child;
  final double maxWidth;

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Align(
        alignment: Alignment.topCenter,
        child: ConstrainedBox(
          constraints: BoxConstraints(maxWidth: maxWidth),
          child: child,
        ),
      ),
    );
  }
}

class SectionHeader extends StatelessWidget {
  const SectionHeader({
    super.key,
    required this.title,
    required this.subtitle,
    required this.icon,
  });

  final String title;
  final String subtitle;
  final IconData icon;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Container(
          padding: const EdgeInsets.all(14),
          decoration: BoxDecoration(
            color: theme.colorScheme.primaryContainer,
            borderRadius: BorderRadius.circular(16),
          ),
          child: Icon(
            icon,
            size: 28,
            color: theme.colorScheme.onPrimaryContainer,
          ),
        ),
        const SizedBox(height: 20),
        Text(title, style: theme.textTheme.headlineMedium),
        const SizedBox(height: 8),
        Text(
          subtitle,
          style: theme.textTheme.bodyMedium?.copyWith(
            color: theme.colorScheme.onSurfaceVariant,
          ),
        ),
      ],
    );
  }
}

class BrandPanel extends StatelessWidget {
  const BrandPanel({super.key, required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(24),
      decoration: BoxDecoration(
        gradient: const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [Color(0xFF172D46), Color(0xFF204B5A)],
        ),
        borderRadius: BorderRadius.circular(24),
      ),
      child: child,
    );
  }
}

Color activityColor(BuildContext context, {required bool active}) {
  final dark = Theme.of(context).brightness == Brightness.dark;
  return active
      ? (dark ? const Color(0xFF6ADBC6) : const Color(0xFF087568))
      : (dark ? const Color(0xFFB1C0CF) : const Color(0xFF526477));
}
