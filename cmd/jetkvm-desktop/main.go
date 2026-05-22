package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lkarlslund/jetkvm-desktop/pkg/gtkui"
	"github.com/lkarlslund/jetkvm-desktop/pkg/logging"
)

const (
	defaultPasswordEnv        = "JETKVM_PASSWORD"
	experimentalUSBNetworkEnv = "JETKVM_DESKTOP_ENABLE_EXPERIMENTAL_USB_NETWORK"
)

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
	cfg := gtkui.Config{}
	logLevel := ""
	passwordFromStdin := false
	passwordEnv := ""

	rootCmd := &cobra.Command{
		Use:   "jetkvm-desktop [base-url-or-host]",
		Short: "Desktop JetKVM client (GTK4)",
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

			if addr := strings.TrimSpace(os.Getenv("JETKVM_DESKTOP_PPROF")); addr != "" {
				go func() {
					log.Printf("[pprof] listening on %s", addr)
					if err := http.ListenAndServe(addr, nil); err != nil {
						log.Printf("[pprof] server error: %v", err)
					}
				}()
			}

			os.Exit(gtkui.Run(cfg))
			return nil
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
