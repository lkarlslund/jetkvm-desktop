# Native Client Plan

## Goal

Build a native desktop client in Go with Ebiten that talks to JetKVM using the same device APIs as the web client, but without a browser.

## What The Existing Device Exposes

Based on `research/kvm`:

- WebRTC session bootstrap is available via `POST /webrtc/session`.
- The request/response payload uses base64-encoded SDP in the `sd` field.
- The client creates these data channels up front:
  - `rpc`
  - `hidrpc`
  - `hidrpc-unreliable-ordered`
  - `hidrpc-unreliable-nonordered`
  - `terminal`
  - `serial` can be added later if needed
- Video is sent as a recv-only H.264 WebRTC track.
- Control/configuration uses JSON-RPC over the `rpc` data channel.
- Keyboard/mouse use the binary HID-RPC protocol over `hidrpc*`.
- HID-RPC starts with a handshake message carrying protocol version `0x01`.
- Device-originated state updates arrive as JSON-RPC requests/events on the `rpc` channel.

Relevant upstream files:

- `research/kvm/web.go`
- `research/kvm/webrtc.go`
- `research/kvm/jsonrpc.go`
- `research/kvm/hidrpc.go`
- `research/kvm/ui/src/routes/devices.$id.tsx`
- `research/kvm/ui/src/hooks/useJsonRpc.ts`
- `research/kvm/ui/src/hooks/useHidRpc.ts`
- `research/kvm/ui/src/hooks/useMouse.ts`
- `research/kvm/ui/src/hooks/useKeyboard.ts`

## Proposed Local Architecture

Use a layered Go client:

1. `internal/deviceauth`
   - Detect whether the device requires login.
   - Support `POST /auth/login-local` and cookie jar persistence.
   - Allow direct local mode when the device is configured as `noPassword`.

2. `internal/signaling`
   - Create the WebRTC offer.
   - Base64-encode local SDP and exchange it with `POST /webrtc/session`.
   - Handle ICE gathering and connection lifecycle.

3. `internal/rtc`
   - Own the `PeerConnection`.
   - Create the required data channels with the exact labels expected by the device.
   - Attach the recv-only H.264 track.

4. `internal/rpc`
   - JSON-RPC client over the `rpc` data channel.
   - Request/response correlation by ID.
   - Event dispatcher for device-pushed messages such as:
     - `videoInputState`
     - `networkState`
     - `usbState`
     - `keyboardLedState`
     - `keysDownState`
     - `otaState`
     - `failsafeMode`
     - `otherSessionConnected`

5. `internal/hidrpc`
   - Implement the binary protocol used by the web client.
   - Support:
     - handshake
     - keypress reports
     - full keyboard reports
     - absolute pointer reports
     - relative mouse reports
     - wheel reports if needed
     - keyboard LED / keys-down state decoding
   - Prefer unreliable channels for high-rate pointer traffic, matching the browser client.

6. `internal/video`
   - Decode/display the incoming H.264 stream.
   - Keep this isolated behind an interface because Ebiten does not decode WebRTC video for us.
   - Expected implementation path:
     - Pion WebRTC receives RTP/video.
     - A Go or CGO-backed H.264 decoder converts frames to RGBA.
     - Ebiten uploads frames into an `ebiten.Image`.

7. `internal/input`
   - Translate Ebiten keyboard/mouse state into JetKVM HID usage codes.
   - Reproduce the browser client’s absolute-coordinate mapping:
     - map visible video area to `0..32767`
     - handle letterboxing/pillarboxing correctly
   - Support relative mouse mode with pointer capture semantics at the app level.

8. `internal/app`
   - Session state machine.
   - Reconnect logic.
   - UI model for overlays, device status, connection state, and settings.

9. `cmd/jetkvm-desktop`
   - Desktop entrypoint.

## Recommended Delivery Phases

### Phase 1: Transport Skeleton

Deliver a binary that can:

- connect to a JetKVM by URL
- authenticate if needed
- establish WebRTC
- open `rpc` and `hidrpc` channels
- perform HID-RPC handshake
- log device RPC events

Success criteria:

- `ping` over JSON-RPC works
- `getVideoState`, `getNetworkSettings`, and `getKeyboardLedState` work
- connection survives reconnect attempts cleanly

### Phase 2: Video Viewer

Deliver a native viewer that:

- renders the H.264 stream in an Ebiten window
- shows connection / no-signal / HDMI-error overlays
- preserves aspect ratio correctly

This phase is the main technical risk because browser media decode is being replaced by an app-managed decode pipeline.

### Phase 3: Input Control

Deliver interactive remote control:

- keyboard input via HID-RPC keypress messages
- absolute mouse mode
- relative mouse mode
- mouse buttons and wheel
- keyboard LED and pressed-key state syncing

Success criteria:

- typing works reliably
- modifiers and key rollover behave correctly
- mouse mapping matches the browser client

### Phase 4: Core Settings And Status

Add the highest-value RPC-backed UI panels first:

- stream quality
- video state
- network summary
- USB state
- reboot / power actions
- keyboard layout
- failsafe mode visibility

### Phase 5: Nice-To-Have Extensions

Only after the core KVM path is stable:

- terminal channel
- serial channel
- file upload / mass storage workflows
- detached windows / multi-monitor improvements
- recording / screenshots

## Suggested Repository Layout

```text
cmd/jetkvm-desktop/
internal/app/
internal/deviceauth/
internal/hidrpc/
internal/input/
internal/rpc/
internal/rtc/
internal/signaling/
internal/video/
docs/
```

## Technical Risks

1. Video decode/rendering is the hardest part.
   Browser playback is currently "free"; native playback is not.

2. Ebiten input semantics differ from the browser.
   Relative mouse capture and keyboard edge cases will need explicit handling.

3. The device currently assumes WebRTC data channels and H.264 video.
   The desktop client should copy those expectations exactly before attempting any protocol changes.

## First Implementation Target

Start with a CLI + minimal windowed prototype:

1. login and session bootstrap
2. `rpc` JSON-RPC client
3. `hidrpc` handshake
4. `ping` and initial state fetch
5. H.264 frame display in a plain Ebiten window
6. keyboard input
7. absolute mouse input

That gives the shortest path to a usable native remote console.
