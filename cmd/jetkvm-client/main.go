package main

import (
	"context"
	"log"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/spf13/cobra"

	"github.com/lkarlslund/jetkvm-native/pkg/app"
)

func main() {
	cfg := app.Config{}

	connectCmd := &cobra.Command{
		Use:   "connect",
		Short: "Connect to a JetKVM device or emulator",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientApp, err := app.New(cfg)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			clientApp.Start(ctx)

			ebiten.SetWindowSize(1280, 720)
			ebiten.SetWindowTitle("jetkvm-client")
			return ebiten.RunGame(clientApp)
		},
	}

	connectCmd.Flags().StringVar(&cfg.BaseURL, "base-url", "http://127.0.0.1:8080", "JetKVM device or emulator base URL")
	connectCmd.Flags().StringVar(&cfg.Password, "password", "", "Password for local auth mode")
	connectCmd.Flags().DurationVar(&cfg.RPCTimeout, "rpc-timeout", 5*time.Second, "Timeout for JSON-RPC requests")

	rootCmd := &cobra.Command{
		Use:   "jetkvm-client",
		Short: "Native JetKVM client",
		RunE: func(cmd *cobra.Command, args []string) error {
			return connectCmd.RunE(cmd, args)
		},
	}
	rootCmd.AddCommand(connectCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
