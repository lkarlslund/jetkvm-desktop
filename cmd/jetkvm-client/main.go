package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/spf13/cobra"

	"github.com/lkarlslund/jetkvm-native/pkg/app"
)

func main() {
	cfg := app.Config{}

	rootCmd := &cobra.Command{
		Use:   "jetkvm-client",
		Short: "Native JetKVM client",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientApp, err := app.New(cfg)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go func() {
				if err := clientApp.Connect(ctx); err != nil && !errors.Is(err, context.Canceled) {
					clientApp.SetStatus(fmt.Sprintf("connect failed: %v", err))
				}
			}()

			ebiten.SetWindowSize(1280, 720)
			ebiten.SetWindowTitle("jetkvm-client")
			return ebiten.RunGame(clientApp)
		},
	}

	rootCmd.Flags().StringVar(&cfg.BaseURL, "base-url", "http://127.0.0.1:8080", "JetKVM device or emulator base URL")
	rootCmd.Flags().StringVar(&cfg.Password, "password", "", "Password for local auth mode")
	rootCmd.Flags().DurationVar(&cfg.RPCTimeout, "rpc-timeout", 5*time.Second, "Timeout for JSON-RPC requests")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
