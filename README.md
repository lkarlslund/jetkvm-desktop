# jetkvm-desktop

Desktop Go client for JetKVM.

## Keyboard Policy

The desktop client sends physical HID usages over `hidrpc`.

- The current client is intentionally physical-key-first, not character-first.
- Letters, modifiers, navigation keys, function keys, and keypad keys are the supported core path.
- Punctuation and non-US layouts are still best-effort in this phase.
- No browser code is reused; protocol behavior is implemented clean-room in this repo.

## Commands

Run the desktop client:

- `jetkvm-desktop connect --base-url http://127.0.0.1:8080`

For local testing, run the emulator harness:

- `go run ./test/emulator-serve serve --listen 127.0.0.1:8080`

The desktop client still defaults to `connect` when invoked without a subcommand.

## Release Builds

GitHub Actions builds native release artifacts on:

- Linux
- macOS
- Windows

Each release contains:

- `jetkvm-desktop`

The release pipeline uses native runners per OS instead of simple Go cross-compilation, because the in-process OpenH264 path uses CGO-backed static libraries.

Create a tag like `v0.1.0` and push it to trigger a combined multi-OS release build.

For a release candidate, use a prerelease tag such as `v0.1.0-rc1`.
