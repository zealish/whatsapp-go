# Zealish WhatsApp Desktop

Native Linux WhatsApp client built on Go + GTK4 + WebKitGTK 6.0. No Electron, no
bundled browser, no reverse-engineered protocol — it embeds the official
WhatsApp Web in the system web engine.

## Requirements

Fedora:

```bash
sudo dnf install gtk4-devel webkitgtk6.0-devel
```

Debian/Ubuntu:

```bash
sudo apt install libgtk-4-dev libwebkitgtk-6.0-dev
```

Go 1.25 or newer.

## Build & Run

```bash
make build
./bin/zealish-wa
```

Install system-wide:

```bash
sudo make install
```

## Layout

| Path | Purpose |
|------|---------|
| `cmd/app` | Entry point, logging setup |
| `internal/app` | Wires window, tray, notifications, bridge |
| `internal/webview` | cgo bridge to GTK4 + WebKitGTK 6.0 |
| `internal/bridge` | JS ↔ Go protocol and injected user script |
| `internal/notify` | Desktop notifications over DBus |
| `internal/tray` | StatusNotifierItem tray icon + DBusMenu |
| `internal/config` | JSON configuration |
| `internal/storage` | XDG path resolution |

## Data Locations

```text
~/.config/zealish-wa/config.json   configuration
~/.local/share/zealish-wa/         session, cookies, IndexedDB
~/.cache/zealish-wa/logs/          logs
```

Set `ZEALISH_WA_DEBUG=1` for debug-level logging.

## Configuration

```json
{
  "window": { "width": 1280, "height": 820, "maximized": false },
  "tray": true,
  "notifications": true,
  "start_minimized": false
}
```

Window geometry is written back on exit.

## Tray Support

The tray uses the StatusNotifierItem protocol. KDE Plasma, XFCE and Cinnamon
support it natively. On GNOME install the
[AppIndicator extension](https://extensions.gnome.org/extension/615/appindicator-support/);
without a StatusNotifierWatcher on the bus the app logs a warning, skips the
tray and the window close button quits normally.

## Security

- No telemetry, no analytics, no intermediate server.
- Session data stays local in WebKit's own storage.
- The JS bridge accepts exactly three message types and never evaluates
  page-supplied code.
- Non-WhatsApp navigations are handed to `xdg-open`.
