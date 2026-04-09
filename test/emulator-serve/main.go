package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/lkarlslund/jetkvm-desktop/pkg/emulator"
)

func main() {
	cfg := emulator.Config{}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the JetKVM emulator test server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.AuthMode == "" {
				cfg.AuthMode = emulator.AuthModeNoPassword
			}
			if cfg.Password != "" {
				cfg.AuthMode = emulator.AuthModePassword
			}

			srv, err := emulator.NewServer(cfg)
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			return srv.ListenAndServe(ctx)
		},
	}

	serveCmd.Flags().StringVar(&cfg.ListenAddr, "listen", "127.0.0.1:8080", "HTTP listen address")
	serveCmd.Flags().StringVar(&cfg.Password, "password", "", "Password for local auth mode; empty enables no-password mode")
	serveCmd.Flags().DurationVar(&cfg.Faults.RPCDelay, "rpc-delay", 0, "Delay every JSON-RPC response by this duration")
	serveCmd.Flags().StringVar(&cfg.Faults.DropRPCMethod, "drop-rpc-method", "", "Drop responses for a specific JSON-RPC method")
	serveCmd.Flags().DurationVar(&cfg.Faults.DisconnectAfter, "disconnect-after", 0, "Disconnect each WebRTC session after this duration")
	serveCmd.Flags().DurationVar(&cfg.Faults.HIDHandshakeDelay, "hid-handshake-delay", 0, "Delay the HID handshake response by this duration")
	serveCmd.Flags().StringVar(&cfg.Faults.InitialVideoState, "video-state", "ready", "Initial video input state event payload")
	serveCmd.Flags().IntVar(&cfg.Width, "width", 960, "Emulated video width")
	serveCmd.Flags().IntVar(&cfg.Height, "height", 540, "Emulated video height")
	serveCmd.Flags().IntVar(&cfg.FPS, "fps", 15, "Emulated video frame rate")

	rootCmd := &cobra.Command{
		Use:   "emulator-serve",
		Short: "JetKVM emulator test harness",
		RunE: func(cmd *cobra.Command, args []string) error {
			return serveCmd.RunE(cmd, args)
		},
	}
	rootCmd.AddCommand(serveCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
