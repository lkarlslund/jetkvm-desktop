package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/lkarlslund/jetkvm-native/pkg/emulator"
)

func main() {
	addr := flag.String("listen", "127.0.0.1:8080", "HTTP listen address")
	password := flag.String("password", "", "Password for local auth mode; empty enables no-password mode")
	rpcDelay := flag.Duration("rpc-delay", 0, "Delay every JSON-RPC response by this duration")
	dropRPCMethod := flag.String("drop-rpc-method", "", "Drop responses for a specific JSON-RPC method")
	disconnectAfter := flag.Duration("disconnect-after", 0, "Disconnect each WebRTC session after this duration")
	hidHandshakeDelay := flag.Duration("hid-handshake-delay", 0, "Delay the HID handshake response by this duration")
	initialVideoState := flag.String("video-state", "ok", "Initial video input state event payload")
	flag.Parse()

	cfg := emulator.Config{
		ListenAddr: *addr,
		AuthMode:   emulator.AuthModeNoPassword,
		Password:   *password,
		Faults: emulator.FaultConfig{
			RPCDelay:          *rpcDelay,
			DropRPCMethod:     *dropRPCMethod,
			DisconnectAfter:   *disconnectAfter,
			HIDHandshakeDelay: *hidHandshakeDelay,
			InitialVideoState: *initialVideoState,
		},
	}
	if *password != "" {
		cfg.AuthMode = emulator.AuthModePassword
	}

	srv, err := emulator.NewServer(cfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.ListenAndServe(ctx); err != nil {
		log.Fatal(err)
	}
}
