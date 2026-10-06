import 'package:flutter/material.dart';

import '../models/device.dart';
import 'screen_layout.dart';

/// A dashboard card showing one PC's name, status indicator and timestamps.
class DeviceCard extends StatelessWidget {
  const DeviceCard({
    super.key,
    required this.device,
    this.onDelete,
    this.onTap,
    this.onLock,
  });

  final Device device;
  final VoidCallback? onDelete;
  final VoidCallback? onTap;
  final VoidCallback? onLock;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final online = device.isOnline;
    final color = activityColor(context, active: online);
    final label = online ? 'On' : 'Offline';

    return Padding(
      padding: const EdgeInsets.only(bottom: 14),
      child: Card(
        child: InkWell(
          onTap: onTap,
          borderRadius: BorderRadius.circular(20),
          child: Padding(
            padding: const EdgeInsets.all(18),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    _ComputerAvatar(color: color, online: online),
                    const SizedBox(width: 14),
                    Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                              device.name,
                              style: theme.textTheme.titleMedium?.copyWith(
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                      const SizedBox(height: 8),
                          _StatusPill(label: label, color: color),
                    ],
                  ),
                    ),
                          if (onDelete != null)
                            PopupMenuButton<String>(
                              icon: const Icon(Icons.more_vert),
                              tooltip: 'Options',
                              shape: RoundedRectangleBorder(
                                borderRadius: BorderRadius.circular(14),
                              ),
                              onSelected: (v) {
                                if (v == 'delete') onDelete!();
                              },
                              itemBuilder: (_) => const [
                                PopupMenuItem(
                                  value: 'delete',
                                  child: ListTile(
                                    leading: Icon(Icons.delete_outline),
                                    title: Text('Remove'),
                                    contentPadding: EdgeInsets.zero,
                                  ),
                                ),
                              ],
                            ),
                  ],
                ),
                      const SizedBox(height: 18),
                      Text(
                        device.statusDetail,
                        style: theme.textTheme.bodyMedium,
                      ),
                      if (device.exactTimestamp.isNotEmpty) ...[
                        const SizedBox(height: 2),
                        Text(
                          device.exactTimestamp,
                          style: theme.textTheme.bodySmall?.copyWith(
                            color: theme.colorScheme.onSurfaceVariant,
                          ),
                        ),
                      ],
                      if (onLock != null || onTap != null) ...[
                        const SizedBox(height: 16),
                        const Divider(height: 1),
                        const SizedBox(height: 8),
                        Wrap(
                          spacing: 8,
                          runSpacing: 4,
                          children: [
                        if (onTap != null)
                          TextButton.icon(
                            onPressed: onTap,
                            icon: const Icon(Icons.history_rounded, size: 20),
                            label: const Text('Activity'),
                          ),
                        if (onLock != null)
                        TextButton.icon(
                          onPressed: onLock,
                          icon: Icon(
                            device.lock?.desiredLocked == true
                                ? Icons.lock_outline
                                : Icons.lock_open_rounded,
                          ),
                          label: Text(device.lock?.summary ?? 'Parental lock'),
                        ),
                          ],
                        ),
                      ],
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _ComputerAvatar extends StatelessWidget {
  const _ComputerAvatar({required this.color, required this.online});
  final Color color;
  final bool online;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 52,
      height: 52,
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.14),
        borderRadius: BorderRadius.circular(16),
      ),
      child: Stack(
        alignment: Alignment.center,
        children: [
          Icon(Icons.desktop_windows_rounded, color: color, size: 26),
          Positioned(
            right: 7,
            top: 7,
            child: _StatusDot(color: color, size: 11),
          ),
        ],
      ),
    );
  }
}

class _StatusDot extends StatelessWidget {
  const _StatusDot({required this.color, this.size = 16});
  final Color color;
  final double size;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        color: color,
        shape: BoxShape.circle,
        border: Border.all(
          color: Theme.of(context).colorScheme.surface,
          width: 2,
        ),
      ),
    );
  }
}

class _StatusPill extends StatelessWidget {
  const _StatusPill({required this.label, required this.color});
  final String label;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        label,
        style: TextStyle(
          color: color,
          fontWeight: FontWeight.w600,
          fontSize: 12,
        ),
      ),
    );
  }
}
