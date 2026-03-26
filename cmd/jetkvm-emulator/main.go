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
	flag.Parse()

	cfg := emulator.Config{
		ListenAddr: *addr,
		AuthMode:   emulator.AuthModeNoPassword,
		Password:   *password,
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
