#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 3 ]; then
  echo "usage: $0 <ui-dir> <emulator-url> <ui-url>" >&2
  exit 1
fi

ui_dir="$1"
emulator_url="$2"
ui_url="$3"

cleanup() {
  if [ -n "${ui_pid:-}" ]; then
    kill "$ui_pid" >/dev/null 2>&1 || true
    wait "$ui_pid" >/dev/null 2>&1 || true
  fi
  if [ -n "${emu_pid:-}" ]; then
    kill "$emu_pid" >/dev/null 2>&1 || true
    wait "$emu_pid" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

go run ./cmd/jetkvm-emulator --listen "${emulator_url#http://}" >/tmp/jetkvm-emulator.log 2>&1 &
emu_pid=$!

for _ in $(seq 1 60); do
  if curl -fsS "${emulator_url}/healthz" >/dev/null; then
    break
  fi
  sleep 1
done
curl -fsS "${emulator_url}/healthz" >/dev/null

pushd "$ui_dir" >/dev/null
npm ci
npx playwright install --with-deps chromium
JETKVM_PROXY_URL="${emulator_url/http/ws}" npx vite dev --mode=device --host 127.0.0.1 --port "${ui_url##*:}" >/tmp/jetkvm-webui.log 2>&1 &
ui_pid=$!

for _ in $(seq 1 60); do
  if curl -fsS "${ui_url}" >/dev/null; then
    break
  fi
  sleep 1
done
curl -fsS "${ui_url}" >/dev/null

node --input-type=module <<'EOF'
import { chromium } from '@playwright/test';

const url = process.env.SMOKE_UI_URL;

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({
  permissions: ['autoplay'],
});

try {
  await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 60000 });
  await page.waitForFunction(() => Boolean(window.__kvmTestHooks), { timeout: 60000 });
  await page.waitForFunction(() => window.__kvmTestHooks?.isWebRTCConnected?.() === true, { timeout: 60000 });
  await page.waitForFunction(() => window.__kvmTestHooks?.isHidRpcReady?.() === true, { timeout: 60000 });
  await page.waitForFunction(() => window.__kvmTestHooks?.isVideoStreamActive?.() === true, { timeout: 60000 });
  const dimensions = await page.evaluate(() => window.__kvmTestHooks?.getVideoStreamDimensions?.() ?? null);
  if (!dimensions || !dimensions.width || !dimensions.height) {
    throw new Error(`expected video stream dimensions, got ${JSON.stringify(dimensions)}`);
  }
  console.log(`Smoke passed: ${JSON.stringify(dimensions)}`);
} finally {
  await browser.close();
}
EOF
popd >/dev/null
