package main

import (
	"context"
	"log"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/spf13/cobra"

	"github.com/lkarlslund/jetkvm-desktop/pkg/app"
)

func main() {
	cfg := app.Config{}

	rootCmd := &cobra.Command{
		Use:   "jetkvm-desktop [base-url-or-host]",
		Short: "Desktop JetKVM client",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				cfg.BaseURL = args[0]
			}

			clientApp, err := app.New(cfg)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			clientApp.Start(ctx)

			ebiten.SetWindowSize(1280, 720)
			ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
			ebiten.SetTPS(ebiten.SyncWithFPS)
			ebiten.SetWindowTitle("jetkvm-desktop")
			return ebiten.RunGame(clientApp)
		},
	}
	rootCmd.Flags().StringVar(&cfg.Password, "password", "", "Password for local auth mode")
	rootCmd.Flags().DurationVar(&cfg.RPCTimeout, "rpc-timeout", 5*time.Second, "Timeout for JSON-RPC requests")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
