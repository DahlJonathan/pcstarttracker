# PC Status

Check your computer's power status (Online / Offline, Booted / Shut down) from
your phone. Three components:

| Component | Tech | Location |
|-----------|------|----------|
| **PC Agent** | Go, Windows Service | [`agent/`](agent) |
| **Backend** | Go REST API + SQLite (WAL) | [`server/`](server) |
| **Mobile App** | Flutter (iOS / Android) | [`lib/`](lib) |

---

## 1. Architecture

```
 ┌────────────┐   pair/init + poll    ┌──────────────┐   list/claim    ┌────────────┐
 │  PC Agent  │ ───────────────────▶  │   Backend    │  ◀───────────── │ Flutter App│
 │ (Windows   │   boot / heartbeat /  │  Go + SQLite │   (JWT user)    │  (phone)   │
 │  service)  │   shutdown events     │              │                 │            │
 └────────────┘  (bearer device tok)  └──────────────┘                 └────────────┘
```

**Pairing (no manual IP/port):**
1. Agent calls `POST /pair/init` with its persistent machine UUID → receives a
   QR token + 6-digit code, which it shows in a local browser page.
2. User signs in on the phone, taps **Add Device**, scans the QR (or types the
   code) → app calls `POST /pair/claim` (authenticated).
3. Backend binds the device UUID to the user and provisions a **permanent device
   token**. The agent retrieves it once via `GET /pair/status` and saves it.
4. Agent runs silently as a Windows service on startup.

**Status rules** (see [`server/internal/status/status.go`](server/internal/status/status.go)):
- `ONLINE` — last heartbeat within **150 s** AND last event is not `shutdown`.
- `OFFLINE` — graceful shutdown recorded, OR heartbeat timed out (sudden power
  loss, crash, sleep), OR never seen.

### Database schema

Entities (see [`server/internal/db/schema.sql`](server/internal/db/schema.sql)):

- **users** `(id, email, password_hash, created_at)`
- **devices** `(id = machine UUID, user_id → users, name, api_token_hash,
  last_event, last_boot_at, last_shutdown_at, last_heartbeat_at, created_at)`
- **device_events** `(id, device_id → devices, event, created_at)` — append-only log
- **pairing_tokens** `(token, code, device_id, device_name, claimed,
  device_token, expires_at, created_at)` — short-lived onboarding sessions

Relationships: a **user** has many **devices**; a **device** has many
**device_events**. Device tokens are stored hashed (SHA-256); passwords use bcrypt.

### API endpoints

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/api/v1/auth/signup` / `login` | — | User JWT |
| POST | `/api/v1/pair/init` | — (fresh token is the secret) | Agent starts pairing |
| GET  | `/api/v1/pair/status?token=` | — | Agent polls for its device token |
| POST | `/api/v1/pair/claim` | user JWT | Phone binds device to account |
| POST | `/api/v1/devices/{id}/events` | device token | `boot` / `shutdown` |
| POST | `/api/v1/devices/{id}/heartbeat` | device token | keep-alive (60 s) |
| GET  | `/api/v1/devices` | user JWT | List devices + computed status |

---

## 2. Run the backend

```powershell
cd server
go run .
```

Environment variables (all optional):

| Var | Default | Notes |
|-----|---------|-------|
| `PC_TRACKER_ADDR` | `:8080` | Listen address |
| `PC_TRACKER_DB` | `pc_tracker.db` | SQLite file (WAL mode) |
| `PC_TRACKER_JWT_SECRET` | `change-me-in-production` | **Set a strong secret in prod** |

Health check: `GET http://localhost:8080/healthz`.

---

## 3. Run the PC agent (Windows)

```powershell
cd agent
# Point at your backend (default http://localhost:8080)
$env:PC_TRACKER_SERVER = "http://<server-host>:8080"

# Foreground: pairs first (opens a browser with QR + code), then reports.
go run . run

# Or build a single binary:
go build -o pc-agent.exe .
```

Install as a background Windows service (run in an **Administrator** prompt):

```powershell
.\pc-agent.exe pair       # one-time pairing
.\pc-agent.exe install    # register auto-start service
.\pc-agent.exe start
```

The service intercepts `SERVICE_CONTROL_SHUTDOWN` / `PRESHUTDOWN` / `STOP` to
send a final `shutdown` event (see
[`agent/internal/winsvc/service_windows.go`](agent/internal/winsvc/service_windows.go)).
Boot events use exponential backoff for slow network init
([`agent/internal/agent/runner.go`](agent/internal/agent/runner.go)).

Config is stored at `%ProgramData%\PCStatusAgent\config.json`.

---

## 4. Run the mobile app

```powershell
flutter pub get
flutter run
```

Set the backend URL from the login screen's settings (⚙) icon. Defaults:
- **Android emulator** → `http://10.0.2.2:8080` (maps to host localhost)
- **Physical phone** → `http://<your-PC-LAN-IP>:8080`

Key files:
- State / API: [`lib/src/state/app_state.dart`](lib/src/state/app_state.dart),
  [`lib/src/api/api_client.dart`](lib/src/api/api_client.dart)
- Dashboard + pull-to-refresh: [`lib/src/screens/dashboard_screen.dart`](lib/src/screens/dashboard_screen.dart)
- Device card: [`lib/src/widgets/device_card.dart`](lib/src/widgets/device_card.dart)
- QR pairing: [`lib/src/screens/pair_screen.dart`](lib/src/screens/pair_screen.dart)

---

## Notes & production hardening

- Serve the backend over **HTTPS** and set a strong `PC_TRACKER_JWT_SECRET`.
  Cleartext HTTP is enabled on Android/iOS only for local-network development.
- The agent's pairing UI uses a local browser page (robust, no native GUI
  toolkit). Swap in a native window (e.g. Fyne) if a standalone GUI is required.
- For higher scale, switch SQLite → PostgreSQL (the `db` package is the only
  file that needs changing) and add a background sweeper if you want push
  notifications on status change.
