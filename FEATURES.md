# JetKVM Desktop — Feature Inventory

Complete specification of every screen, panel, overlay, button, toggle, text field,
keyboard shortcut, and application state.  This document is the contract for the
GTK4 rewrite.

---

## Application States (top-level)

| State | Condition | What is drawn |
|---|---|---|
| Launcher — Browse | `launcherOpen && mode == browse` | Device discovery list + manual connect |
| Launcher — Password | `launcherOpen && mode == password` | Password prompt for pending target |
| Session view | connected controller, no WoL takeover | Scaled remote video + chrome + overlays |
| WoL takeover | `wolOpen` while connected | Full-screen Wake-on-LAN panel; video hidden |
| Empty | no controller, launcher closed | Blank themed background |

### Session sub-states

- **Connection phase** (`session.Phase`): `Connecting`, `Reconnecting`, `Connected`, `Disconnected`, `AuthFailed`, `OtherSession`, `Rebooting`, `Fatal`
- **Overlay flags** (at most one modal open): `settingsOpen`, `pasteOpen`, `statsOpen`, `mediaOpen`, `serialConsoleOpen`
- **Mouse mode**: absolute / relative
- **Total capture**: input grabber active/inactive
- **Chrome visibility**: auto-hide unless pinned, overlay open, or cursor in reveal zone
- **Paste in progress**: banner shown, keyboard to remote blocked

---

## 1. Launcher

### 1A. Browse Mode

**Header**
- Title: "JetKVM"
- Subtitle: "Available devices on your local network"

**Discovery list** (max 7 rows)
- Empty state: "Scanning local subnets…" + explanation
- Per device row (clickable):
  - Device name, base URL, status ("Configured" / "Needs setup")
- Footer: "Updated X ago"

**Recent connections** (max 3)
- Per row: name/URL + age + **X** delete button
- Click row → connect
- Click X → remove from recents

**Manual connect**
- Text field (placeholder: `jetkvm.local or 192.168.1.50`)
- Button: **Connect** (enabled when host valid)
- Validation error display

### 1B. Password Mode

- Target URL label
- Password field (masked, placeholder "Local password")
- Button: **Back** → return to browse
- Button: **Connect** (enabled when password non-empty)
- Error text on auth failure

---

## 2. Session View — Video Area

- Letterboxed, aspect-ratio-preserved scaled remote video frame
- Empty background when no frame received yet

---

## 3. Connection Status Banner (top-right overlay)

Shown when phase != connected-ready or video not ready:

| Phase | Title | Detail | Button |
|---|---|---|---|
| Connecting | Connecting | Opening auth, WebRTC, HID | — |
| Reconnecting | Reconnecting | status text | — |
| AuthFailed | Authentication Failed | Check password/auth | — |
| OtherSession | Session Replaced | Another client took over | **Take Back Control** |
| Rebooting | Rebooting | Waiting for device | — |
| Disconnected | Disconnected | Session dropped | — |
| Fatal | Fatal Error | error text | — |
| Connected, no video | Loading Video | Waiting for first frame | — |

---

## 4. Chrome Toolbar

Auto-hides unless pinned, overlay open, or cursor in reveal zone.
Draggable to any screen edge.  Click drag handle without drag → flip H/V layout.
Tooltips on hover.

| Icon | Tooltip | When shown | Action |
|---|---|---|---|
| reconnect | Reconnect / Retry / Connect | Not connected | Reconnect session |
| paste | Paste text | Connected | Toggle paste overlay + load clipboard |
| media | Virtual media | Connected | Toggle media overlay |
| serial_console | Serial console | Connected + serial extension active | Toggle serial console |
| wol | Wake on LAN | Connected | Toggle WoL full-screen |
| capture | Total Capture | Connected + grabber supported | Toggle input grabber |
| stats | Connection stats | Always | Toggle stats panel |
| fullscreen | Toggle fullscreen | Always | Toggle fullscreen |
| settings | Settings | Always | Toggle settings overlay |
| chrome_drag | Drag to move | Always | Drag to reposition; click to flip H/V |

Chrome position persists via `ChromeCustomX`, `ChromeCustomY`, `ChromeCustomPos` prefs.

---

## 5. Status Footer

- Left: `RTC … HID … Video …` (ready/pending indicators)
- Right: trimmed error when not connected
- Hidden when `HideStatusBar` pref is true
- Fades with chrome alpha

---

## 6. Settings Hint

- Small settings icon bottom-right, shown ~10 seconds after first connect
- Click → open settings

---

## 7. Paste-in-Progress Banner

- Text: "Pasting — input is disabled until complete"
- Button: **Cancel** → cancel remote paste RPC

---

## 8. Pressed Keys Overlay

- When `ShowPressedKeys` pref enabled and keys held
- Shows `Keys: …` bottom-left

---

## 9. Stats Overlay

Panel (top-right), toggled by stats chrome button.

**Live metrics (text)**
- Signaling mode, RTC state, HID ready, Video ready
- Resolution, Quality %, Frame age
- Bitrate, Decode FPS, Jitter, RTT, Packets lost, Error

**Graphs (2-minute rolling history)**
- Bitrate (kbps)
- Jitter (ms)
- RTT (ms)
- Decode FPS

---

## 10. Paste Overlay

Modal dialog.

- Description of HID macro paste behavior
- Keyboard Layout (read-only from session)
- Delay (read-only, 100 ms default)
- Text area (placeholder or pasted content)
- Skipped characters warning (if unsupported runes)
- Error / "Paste in progress…" status

**Buttons**
- **Load Clipboard**
- **Cancel** → close + cancel in-progress paste
- **Send** → execute paste macro (disabled if empty or in progress)

Backdrop click → dismiss.

---

## 11. Serial Console Overlay

Modal dialog.

- Status: Connected / extension inactive / waiting / error
- Scrollable output buffer
- Hint: typing sent to serial; Esc closes
- Button: **X** → close

**Input handling**
- Typed characters → serial
- Enter → send terminator
- Backspace → send `\x7f`
- Tab → send `\t`
- Mouse wheel → scroll output
- Esc → close

---

## 12. Virtual Media Overlay

Modal dialog with tabs.

**Header**
- Title: "Virtual Media"
- "Working…" spinner when loading
- Button: **X** → close (disabled during upload)

**Current mount card**
- "Nothing mounted" or: Source, Mode, filename/URL, size
- Button: **Unmount** (when mounted)

**Tabs: Overview | URL | Storage | Upload**

### Tab: Overview
- Device name, Storage used/free
- Tips about ISO/IMG modes

### Tab: URL
- URL text field
- USB Mode toggle: **CD/DVD** | **Disk**
- Button: **Mount URL**
- Validation error for invalid URL

### Tab: Storage
- File list (max 7 rows): click to select, **Delete** per file
- USB Mode toggle + Button: **Mount Selected**
- Used/Free summary

### Tab: Upload
- Path text field + Button: **Browse** (native file dialog: `.iso`, `.img`)
- Mount Mode toggle: **CD/DVD** | **Disk**
- Button: **Start Upload**
- Progress bar with bytes/s and ETA when uploading

Backdrop click → dismiss (unless uploading).

---

## 13. Wake-on-LAN Screen

Full-screen modal (replaces session draw while open).

**Header**: Wake On LAN + description

**Saved devices list**
- Per device: name, MAC
- Button: **Wake** → send magic packet
- Button: **X** → arm delete confirm:
  - "Delete this device?" + **Yes, Delete** | **Cancel**

**Add new device**
- Name text field
- MAC Address text field
- Button: **+ Add Device**

**Button: Close**

Messages: loading, error, success.

---

## 14. Settings Overlay

Left sidebar (12 sections) + content panel + **X** close button.

### 14.1 General

**Device card** (read-only): Base URL, Phase, Signaling mode

**Updates card**
- Current / detected latest (App + System versions)
- Button: **Check for updates**
- Button: **Install updates** (if available)
- Toggle: **Auto updates** (Enabled label)
- Status spinners/errors

**Actions card**
- Button: **Reconnect** / **Retry** / **Connect** (phase-dependent)
- Button: **Reboot device**

### 14.2 Mouse

**Pointer card**
- Toggle: **Absolute** | **Relative** mode
- Toggle: **Hide Host Cursor**
- Slider: **Wheel Throttle** (0–max ms)
- Slider: **Movement Throttle** (0–max ms)
- Toggle: **Invert Scroll**
- Toggle: **Reroute Side Buttons in Absolute Mode**

**Jiggler card**
- State, Preset readouts
- Presets: **Disabled**, **Frequent**, **Standard**, **Light**, **Custom**
- Custom editor (when open):
  - Idle (s) ± buttons
  - Jitter % ± buttons
  - Cron text field, Timezone text field
  - Button: **Save Custom** | **Cancel**

### 14.3 Keyboard

- Toggle: **Show Pressed Keys**
- Total Capture toggle key selector: F1–F12, Pause, ScrollLock
- Toggle: **Enable Experimental Remote Hotkeys**
- Backend label (e.g. `ebiten (window)`)
- Layout presets (19 layouts): cs-CZ, da-DK, de-CH, de-DE, en-UK, en-US, es-ES, nl-BE, fr-CH, fr-FR, hu-HU, it-IT, ja-JP, nb-NO, pl-PL, pt-PT, sv-SE, sl-SI, ru-RU
- Shortcut chord reference: Ctrl+Alt+` → Alt+Tab; Ctrl+Alt+Shift+` → Shift+Alt+Tab

### 14.4 Video

**Stream card**
- Quality: **High** | **Medium** | **Low**
- Current quality factor readout
- Codec: **Auto** | **H265** | **H264**

**EDID card**
- Current EDID hex display
- Presets: **JetKVM**, **Acer**, **ASUS**, **Dell**, **iDRAC**
- Custom: summary + Button: **Load from Clipboard** | **Clear** | **Apply Custom**

**H265 Not Supported modal**
- Button: **Switch To H265** | **Cancel**

### 14.5 Hardware

**Display card**
- Rotation: **Normal** | **Inverted**
- Brightness: **Off**, **Low**, **Medium**, **High**
- Dim after: Never, 1m, 5m, 10m, 30m, 1h
- Turn off after: Never, 5m, 10m, 30m, 1h
- Toggle: **HDMI Sleep Power Saving**

**USB card**
- Toggle: **Enable USB Emulation**
- Advertised device / USB Profile readouts
- Quick profiles: **Keyboard + Mouse + Storage** | **Keyboard Only**
- Custom capability toggles: Keyboard, Absolute Mouse, Relative Mouse, Virtual Media, Serial Console, USB Network Adapter

**USB Network card** (experimental)
- Toggle: **Enable USB Network Gadget**
- Host preset: Auto, Linux, macOS, Windows, Custom
- Protocol: ECM, NCM, RNDIS
- Sharing: NAT, Bridge
- Uplink: Auto, Manual + interface text field
- IPv4 subnet CIDR text field
- Toggle: **Enable DHCP**
- Toggle: **Enable DNS Proxy**
- Button: **Save USB Network**

### 14.6 Extension

Extension selector: **None** | **ATX** | **DC** | **Serial**

**ATX Power card** (when ATX active)
- Power LED / HDD LED indicators
- Button: **Power** (short press)
- Button: **Reset**
- Button: **Long Press**
- Confirm modals: **Reset Host** / **Force Off Host** / **Continue** + **Cancel**

**DC Power card** (when DC active)
- Button: **Power On** | **Power Off**
- Restore on power loss: **Off** | **On** | **Last**
- Voltage / Current / Power readouts

**Serial Console card** (when Serial active)
- Button: **Open Console**
- Baud rate: 9600, 19200, 38400, 57600, 115200
- Data bits: 7, 8
- Stop bits: 1, 1.5, 2
- Parity: None, Even, Odd, Mark, Space
- Terminator: None, CR, LF, CRLF, LFCR
- Toggles: Hide Serial Settings, Local Echo, Preserve ANSI, Show Newline Tag
- Normalization: Caret, Names, Hex
- Quick commands (dynamic from device config)
- Recent commands list

### 14.7 Access

**Local Access card**
- Authentication mode, Loopback Only readout
- Button: **Change Password** | **Disable Password** (when password mode)
- Button: **Enable Password** (when no password)

**TLS card**
- Mode readout
- Mode selector: **Disabled** | **Self-Signed** | **Custom**
- Certificate / Private key summaries
- Button: **Load from Clipboard** | **Clear** (each)
- Button: **Apply Custom TLS**

**Cloud card** (read-only): Connected, Cloud API URL, Cloud App URL

**Access editor modals** (stacked on settings)
- Enable / Change / Disable password forms with masked fields
- Button: **Save Password** / **Update Password** / **Disable Password** + **Cancel**

### 14.8 Appearance

- Theme: **System** | **Dark** | **Light**
- Toggle: **Always visible** (pin chrome)
- Toggle: **Hide Footer Status**
- Button: **Toggle Fullscreen**
- Initial window after connect: **Unchanged** | **Maximize** | **1:1 pixel ratio** | **Fullscreen**
- Button: **Reset position** (chrome to default)
- Help text about drag handle

### 14.9 Macros

**Library card**
- Button: **Add Macro**
- Per macro row: **Up**, **Down**, **Edit**, **Duplicate**, **Delete**

**Editor card**
- Macro name text field
- Step navigation: **Previous**, **Next**, **Add Step**, **Remove Step**, **Move Up**, **Move Down**
- Modifiers text field, Keys text field, Delay (ms) text field
- Button: **Save Macro** | **Cancel**

### 14.10 Network

**Editable Settings card**
- Hostname, Domain, HTTP Proxy text fields
- IPv4 Mode: **DHCP** | **Static** | **Disabled** (+ static address fields)
- IPv6 Mode: **SLAAC** | **Static** | **Disabled** (+ static fields)
- mDNS: Auto, Disabled, IPv4 Only, IPv6 Only
- Time Sync: NTP Only, NTP+HTTP, HTTP Only, Custom (+ NTP/HTTP URL fields)
- Button: **Save Settings** | **Renew DHCP Lease**

**Current State card** (read-only diagnostics)
- Button: **Refresh**

**DHCP Lease card** (read-only)

**Public Reachability card**
- Public IP list, Tailscale status block

### 14.11 MQTT

**Configuration card**
- Toggle: **Enable MQTT**
- Text fields: Broker, Port, Username, Password, Base Topic
- Toggles: **Use TLS**, **Allow Insecure TLS**, **Home Assistant Discovery**, **Enable MQTT Actions**
- Debounce (ms) text field
- Button: **Test Connection** | **Save Settings**

**Status card** (read-only): Connected, Broker, Base Topic, TLS, Actions, errors

### 14.12 Advanced

Read-only: Developer Mode, Dev Channel, Loopback Only, USB Emulation, versions

Toggles (when available):
- **Use Development Channel**
- **Loopback Only**
- **Developer Mode**

SSH Authorized Key text field + Button: **Save SSH Key**

**Factory Reset**
- Button: **Factory Reset** → confirm panel:
  - Button: **Confirm Reset** | **Cancel**

---

## 15. Keyboard Shortcuts

| Shortcut | Context | Action |
|---|---|---|
| Esc | Any overlay open | Close that overlay |
| Enter | Launcher browse (valid host) | Connect |
| Enter | Launcher password | Connect with password |
| Enter | WoL overlay | Add device |
| Tab | WoL overlay | Toggle focus Name / MAC |
| Enter | Media URL tab (URL focused) | Mount URL |
| Enter | Media Upload tab (path focused) | Start upload |
| Ctrl+Enter | Paste overlay | Send paste |
| Ctrl+V | Paste overlay | Load clipboard |
| Enter | Paste overlay | Insert newline |
| Backspace | Paste overlay | Delete char |
| Enter/Numpad Enter | Serial console | Send terminator |
| Backspace | Serial console | Send `\x7f` |
| Tab | Serial console | Send `\t` |
| Mouse wheel | Serial console | Scroll output |
| Capture toggle key | Session (no overlay) | Toggle fullscreen + Total Capture |
| Ctrl+Alt+` | Session (experimental hotkeys) | Remote Alt+Tab |
| Ctrl+Alt+Shift+` | Session (experimental hotkeys) | Remote Shift+Alt+Tab |
| Tab | Settings section forms | Cycle focused text fields |
| Enter | Settings section forms | Save/submit form |

---

## 16. User Preferences

Persisted in `~/.config/jetkvm-desktop/preferences.json`:

| Field | UI Surface | Default |
|---|---|---|
| Theme | Appearance | system |
| PinChrome | Appearance → Always visible | false |
| HideHeaderBar | (stored, not wired) | false |
| HideStatusBar | Appearance → Hide Footer Status | false |
| ChromeAnchor | Reset position / drag | top_right |
| ChromeLayout | Drag handle click | horizontal |
| ChromeCustomX/Y/Pos | Drag reposition | — |
| HideCursor | Mouse → Hide Host Cursor | false |
| InvertScroll | Mouse → Invert Scroll | false |
| ShowPressedKeys | Keyboard → Show Pressed Keys | false |
| ExperimentalGlobalHotkeys | Keyboard → Enable Experimental | false |
| AbsoluteSideButtonsViaRel | Mouse → Reroute Side Buttons | true |
| ScrollThrottleMs | Mouse → Wheel Throttle slider | 0 |
| PointerMoveThrottleMs | Mouse → Movement Throttle slider | 8 |
| CaptureToggleKey | Keyboard → Toggle key | ScrollLock |
| ConnectWindowMode | Appearance → Initial window | maximize |
| RecentConnections | Launcher recents | [] (max 10) |

---

## 17. Window Management

- Launcher default size: 480 x 640
- Session target size: 1920 x 1080 (capped at 90% of monitor)
- On connect: apply `ConnectWindowMode` pref (unchanged / maximize / 1920x1080 / fullscreen)
- Browse → session transition: expand from launcher size

---

## 18. Theme

- System / Dark / Light selection
- System detection: GNOME gsettings → macOS defaults → Windows registry → fallback dark
- For GTK4 rewrite: use `Gtk.Settings` prefer-dark-theme + custom CSS provider
