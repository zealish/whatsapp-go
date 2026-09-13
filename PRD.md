# PRD — Zealish WhatsApp Desktop

Version: 1.0  
Status: Draft (MVP)  
Author: Zealish  
License: Proprietary (dapat diubah menjadi Open Source)  

> Aplikasi WhatsApp Desktop berbasis **Golang + Native WebView** dengan fokus utama pada **stabilitas, efisiensi resource, dan pengalaman native Linux**. Bukan Electron, bukan browser custom, dan tidak menggunakan reverse-engineered protocol seperti WhatsMeow.

---

# 1. Product Vision

Membangun aplikasi desktop WhatsApp yang ringan seperti aplikasi native, namun tetap menggunakan **WhatsApp Web resmi** sebagai backend sehingga perubahan protokol dari Meta tidak menyebabkan aplikasi mudah rusak.

Prinsip utama:

- Stability First
- Memory Efficient
- Native Experience
- Long-term Maintainability

---

# 2. Objectives

### Primary Goals

- Menggunakan WebView sistem (WebKitGTK)
- Startup < 1 detik
- Idle CPU < 1%
- RAM Go < 25 MB
- Binary < 12 MB
- Session persisten
- Native notification
- Native system tray

### Success Metrics

| Metric | Target |
|---------|--------|
| Cold startup | <1s |
| Go RSS | <25 MB |
| Binary size | <12 MB |
| Idle CPU | <1% |
| Crash rate | Mendekati 0 |

---

# 3. Target Platform

| OS | Support |
|-----|---------|
| Fedora 42+ | ✅ Primary |
| Ubuntu 24.04 | ✅ |
| Debian 13 | ✅ |
| Arch Linux | ✅ |
| Windows | Future |
| macOS | Future |

MVP diprioritaskan Linux Wayland.

---

# 4. Technical Stack

| Layer | Technology |
|--------|------------|
| Language | Go 1.25+ |
| Window | GTK4 |
| Web Engine | WebKitGTK |
| WebView | webview_go |
| Notifications | DBus |
| Tray | Ayatana AppIndicator |
| Config | JSON |
| Build | Go Modules |

Tidak ada:

- Electron
- Node.js Runtime
- Chromium Embedded Framework
- Tauri

---

# 5. Architecture

```text
                   User

                    │

         Native GTK4 Window

                    │

        WebView (WebKitGTK)

                    │

     https://web.whatsapp.com

                    │

──────────── JavaScript Bridge ────────────

                    │

              Go Core Runtime

   ┌─────────────┬──────────────┐

   │             │              │

Window      Notification     Tray

Manager       Manager       Manager

   │             │              │

 Config      DBus Linux     AppIndicator
```

Semua logika desktop berada di Golang.

WhatsApp tetap dijalankan oleh WebKitGTK.

---

# 6. Why WebView?

### Dibanding Electron

| Electron | Native WebView |
|----------|----------------|
| Bundle Chromium | Pakai WebKit sistem |
| 180–300 MB | Sangat kecil |
| Startup lebih lambat | Sangat cepat |
| Binary besar | Binary kecil |

### Dibanding WhatsMeow

| WhatsMeow | WebView |
|-----------|----------|
| Reverse engineered | Official Web |
| Perlu update protocol | Stabil |
| UI harus dibuat | UI resmi |
| Risiko incompatibility | Lebih rendah |

Keputusan: **Native WebView**.

---

# 7. Core Features

## 7.1 Login

Fungsi:

- Membuka WhatsApp Web
- Scan QR
- Auto restore session

Acceptance:

- QR tampil otomatis
- Tidak login ulang setelah restart

---

## 7.2 Persistent Session

Lokasi data:

```text
~/.local/share/zealish-wa/
```

Struktur:

```text
cookies.db
localstorage/
indexeddb/
cache/
```

Menggunakan storage bawaan WebKit.

---

## 7.3 Native Window

Fitur:

- Resize
- Remember position
- Remember size
- Maximize
- Minimize
- Frameless: No (gunakan native decoration)

---

## 7.4 System Tray

Menu:

- Open
- Reload
- Toggle Notification
- Quit

Behavior:

- Tombol X → Hide to tray
- Quit → benar-benar keluar

---

## 7.5 Desktop Notification

Menggunakan DBus.

Isi notifikasi:

- Nama pengirim
- Preview pesan
- Avatar (jika tersedia)

Klik notifikasi membuka chat terkait.

---

## 7.6 External Browser

Semua link selain WhatsApp dibuka menggunakan browser default sistem.

Tidak membuka tab baru di WebView.

---

# 8. JavaScript Bridge

Bridge dibuat sekecil mungkin.

### JS → Go

```go
Notify(title, body)
OpenExternal(url)
Ready()
```

### Go → JS

```go
Reload()
FocusChat(chatID)
```

Tidak mengizinkan arbitrary JavaScript execution.

---

# 9. Configuration

Lokasi:

```text
~/.config/zealish-wa/config.json
```

Contoh:

```json
{
  "window": {
    "width": 1280,
    "height": 820
  },
  "tray": true,
  "notifications": true,
  "start_minimized": false
}
```

---

# 10. Folder Structure

```text
zealish-whatsapp/

├── cmd/
│   └── app/
│       └── main.go
│
├── internal/
│   ├── app/
│   ├── window/
│   ├── webview/
│   ├── tray/
│   ├── notify/
│   ├── bridge/
│   ├── config/
│   └── storage/
│
├── assets/
│   ├── icon.svg
│   ├── icon.png
│   └── desktop.desktop
│
├── packaging/
│   ├── rpm/
│   └── flatpak/
│
├── configs/
│
├── go.mod
└── README.md
```

---

# 11. Storage Design

### Config

```text
~/.config/zealish-wa/
```

### Runtime Data

```text
~/.local/share/zealish-wa/
```

### Logs

```text
~/.cache/zealish-wa/logs/
```

Log maksimal 5 file × 5 MB (rotating).

---

# 12. Security

### Privacy

- Tidak ada telemetry
- Tidak ada analytics
- Tidak ada akun Zealish
- Tidak ada server perantara

### Session

Session hanya disimpan lokal menggunakan WebKit encrypted cookie storage bila tersedia.

---

# 13. Performance Strategy

## Memory

Hanya satu WebView.

Tidak ada multi-process tambahan dari aplikasi Go.

## CPU

- Event-driven
- Tidak polling DOM
- Notification menggunakan MutationObserver seperlunya

## Startup

Load sequence:

1. GTK init
2. Window
3. WebView
4. Restore cookies
5. Load WhatsApp

Target total <1 detik.

---

# 14. Packaging

## RPM

```bash
sudo dnf install zealish-wa.rpm
```

## Flatpak

App ID

```text
com.zealish.WhatsApp
```

---

# 15. CI/CD

GitHub Actions:

Pipeline:

- gofmt
- govet
- golangci-lint
- unit test
- build rpm
- build flatpak
- release artifact

---

# 16. Milestones

## Phase 1

- GTK window
- WebView
- WhatsApp loading

## Phase 2

- Session persistence
- Config
- Window state

## Phase 3

- Tray
- Notification
- External links

## Phase 4

- RPM
- Flatpak
- Auto update checker (tanpa auto install)

---

# 17. Coding Standards

- Clean Architecture
- Internal package only
- Zero global mutable state
- Context-aware goroutines
- Graceful shutdown
- No panic in production
- golangci-lint harus 100% clean

---

# 18. Future Roadmap

### v1.1

- Download manager
- Drag & drop upload
- Badge unread counter

### v1.2

- Multi window
- Multiple profiles

### v2.0

- Windows support
- macOS support
- Native updater
