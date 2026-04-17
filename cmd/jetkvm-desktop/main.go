package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/spf13/cobra"

	"github.com/lkarlslund/jetkvm-desktop/pkg/app"
	"github.com/lkarlslund/jetkvm-desktop/pkg/logging"
)

const defaultPasswordEnv = "JETKVM_PASSWORD"
const experimentalUSBNetworkEnv = "JETKVM_DESKTOP_ENABLE_EXPERIMENTAL_USB_NETWORK"

func readPassword(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}

func resolvePassword(passwordFromStdin bool, passwordEnv string, stdin io.Reader, getenv func(string) string) (string, error) {
	switch {
	case passwordFromStdin && passwordEnv != "":
		return "", errors.New("--password-stdin and --password-env cannot be used together")
	case passwordFromStdin:
		return readPassword(stdin)
	case passwordEnv != "":
		return getenv(passwordEnv), nil
	default:
		return getenv(defaultPasswordEnv), nil
	}
}

func envEnabled(name string, getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv(name))) {
	case "1", "true":
		return true
	default:
		return false
	}
}

func main() {
	cfg := app.Config{}
	logLevel := ""
	passwordFromStdin := false
	passwordEnv := ""

	rootCmd := &cobra.Command{
		Use:   "jetkvm-desktop [base-url-or-host]",
		Short: "Desktop JetKVM client",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				cfg.BaseURL = args[0]
			}
			password, err := resolvePassword(passwordFromStdin, passwordEnv, os.Stdin, os.Getenv)
			if err != nil {
				return fmt.Errorf("resolve password: %w", err)
			}
			cfg.Password = password

			if err := logging.Configure(logLevel); err != nil {
				return err
			}
			cfg.ExperimentalUSBNetwork = envEnabled(experimentalUSBNetworkEnv, os.Getenv)

			clientApp, err := app.New(cfg)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			clientApp.Start(ctx)

			windowWidth, windowHeight := app.InitialWindowSize(cfg.BaseURL == "")
			ebiten.SetWindowSize(windowWidth, windowHeight)
			ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
			ebiten.SetTPS(ebiten.SyncWithFPS)
			ebiten.SetWindowTitle("jetkvm-desktop")
			return ebiten.RunGame(clientApp)
		},
	}
	rootCmd.Flags().BoolVar(&passwordFromStdin, "password-stdin", false, "Read password for local auth mode from stdin")
	rootCmd.Flags().StringVar(&passwordEnv, "password-env", "", fmt.Sprintf("Read password for local auth mode from the named environment variable (default fallback: %s)", defaultPasswordEnv))
	rootCmd.Flags().StringVar(&logLevel, "log-level", "", fmt.Sprintf("Log level: error, warn, info, debug, trace (default: error; env: JETKVM_DESKTOP_LOG_LEVEL; experimental USB network UI env: %s)", experimentalUSBNetworkEnv))
	rootCmd.Flags().DurationVar(&cfg.RPCTimeout, "rpc-timeout", 5*time.Second, "Timeout for JSON-RPC requests")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
