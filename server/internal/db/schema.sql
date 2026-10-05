PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

-- Mobile app users.
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    email         TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    created_at    TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- A paired PC. `id` is the agent-generated persistent machine UUID.
CREATE TABLE IF NOT EXISTS devices (
    id                TEXT    PRIMARY KEY,                       -- machine UUID
    user_id           INTEGER REFERENCES users(id) ON DELETE CASCADE,
    name              TEXT    NOT NULL DEFAULT 'My PC',
    api_token_hash    TEXT,                                     -- hash of the permanent device token
    last_event        TEXT,                                     -- 'boot' | 'shutdown'
    last_boot_at      TEXT,
    last_shutdown_at  TEXT,
    last_heartbeat_at TEXT,
    created_at        TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_devices_user ON devices(user_id);

-- Append-only audit log of device lifecycle events.
CREATE TABLE IF NOT EXISTS device_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id  TEXT    NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    event      TEXT    NOT NULL,                                 -- 'boot' | 'shutdown' | 'heartbeat'
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_events_device ON device_events(device_id, created_at);

-- Short-lived pairing session created by the PC agent, claimed by the mobile app.
CREATE TABLE IF NOT EXISTS pairing_tokens (
    token      TEXT    PRIMARY KEY,                              -- opaque QR token
    code       TEXT    NOT NULL,                                 -- 6-digit human code
    device_id  TEXT    NOT NULL,                                 -- machine UUID being paired
    device_name TEXT   NOT NULL DEFAULT 'My PC',
    claimed    INTEGER NOT NULL DEFAULT 0,
    device_token TEXT,                                           -- permanent token, held until agent polls once
    expires_at TEXT    NOT NULL,
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_pairing_code ON pairing_tokens(code);

-- Desired parental restriction and the last agent acknowledgement.
CREATE TABLE IF NOT EXISTS device_locks (
    device_id TEXT PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    revision INTEGER NOT NULL DEFAULT 0,
    desired_locked INTEGER NOT NULL DEFAULT 0,
    password_hash TEXT NOT NULL DEFAULT '',
    applied_revision INTEGER NOT NULL DEFAULT -1,
    applied_locked INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    confirmed_at TEXT
);
