# jetkvm-native

Native Go client for JetKVM.

## Commands

Run the native client:

- `jetkvm-client connect --base-url http://127.0.0.1:8080`

Run the emulator:

- `jetkvm-emulator serve --listen 127.0.0.1:8080`

For backward compatibility, both binaries still default to those subcommands when invoked without one.

## Release Builds

GitHub Actions builds native release artifacts on:

- Linux
- macOS
- Windows

Each release contains:

- `jetkvm-client`
- `jetkvm-emulator`

The release pipeline uses native runners per OS instead of simple Go cross-compilation, because the in-process OpenH264 path uses CGO-backed static libraries.

Create a tag like `v0.1.0` and push it to trigger a combined multi-OS release build.
