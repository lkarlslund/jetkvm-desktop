# Native UI Notes

The desktop client should preserve as much video area as possible. The current direction is:

- full-screen video with minimal margins
- a compact top action bar that fades away while idle
- a settings overlay whose section structure mirrors the upstream web UI

Upstream web UI top-level controls currently include:

- paste text, with OCR/copy-text as a split action
- virtual media
- wake-on-LAN
- virtual keyboard
- extensions
- connection stats
- settings
- detach window
- fullscreen

Upstream web UI settings sections currently include:

- General
- Mouse
- Keyboard
- Video
- Hardware
- Access
- Appearance
- Keyboard Macros
- Network
- MQTT
- Advanced

Within those sections, the current upstream feature surface includes:

- General: locale, check for updates, auto-update, reboot
- Mouse: cursor hide, scroll throttling, jiggler, absolute/relative mode
- Keyboard: layout, pressed-key display
- Video: stream quality, EDID presets/custom EDID, client-side image tuning
- Hardware: display rotation, backlight, power saving, USB device classes, USB identifiers
- Access: local auth, TLS mode/certs, cloud adoption/deregistration
- Appearance: theme
- Macros: create/edit/reorder/delete keyboard macros
- Network: DHCP/static IPv4/IPv6, DNS, domain, lease info, public IP, tailscale
- MQTT: broker/TLS/topic/discovery/actions
- Advanced: developer mode, dev channel, USB emulation, loopback-only, SSH key, reset config, custom version update

The desktop client does not need web parity immediately, but its settings IA should match these sections so users do not have to relearn the product.
