# jetkvm-desktop

A native desktop client for JetKVM with local discovery, direct connect, remote control, and core settings in one window.

![jetkvm-desktop launcher](docs/launcher.png)

## What It Does

- Finds JetKVM devices on your local network
- Connects directly by hostname, mDNS name, or IP
- Shows the remote video feed in a native desktop window
- Sends keyboard and mouse input to the target machine
- Prompts for a password when the device requires it
- Exposes the main settings and connection stats without opening the browser UI

## Getting Started

Open the launcher:

```bash
jetkvm-desktop
```

Connect straight to a known device:

```bash
jetkvm-desktop jetkvm.local
jetkvm-desktop 192.168.1.50
jetkvm-desktop http://192.168.1.50
```

If the device requires a password, the app will ask for it.

## Inside the App

![jetkvm-desktop settings](docs/settings.png)

Once connected, the app keeps video, input, stats, and the core device settings in the same window, so the common JetKVM workflow stays fast and desktop-native.

## JetKVM

This is a separate desktop client for the JetKVM ecosystem. For the upstream JetKVM repositories, see `github.com/jetkvm`.

## Downloads

Prebuilt releases are published on GitHub Releases:

`https://github.com/lkarlslund/jetkvm-desktop/releases`
