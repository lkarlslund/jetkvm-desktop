// Command jetkvm-desktop is the Linux JetKVM client.
//
// During the Gio rewrite this binary is intentionally minimal: it stands up a
// Gio window and reports paint statistics so we can validate the rendering
// stack independently of the existing pkg/app code (which still depends on
// Ebiten and is being replaced incrementally). Backend wiring is reintroduced
// once enough of the new presentation layer exists.
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

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/spf13/cobra"

	"github.com/lkarlslund/jetkvm-desktop/pkg/logging"
)

const (
	defaultPasswordEnv        = "JETKVM_PASSWORD"
	experimentalUSBNetworkEnv = "JETKVM_DESKTOP_ENABLE_EXPERIMENTAL_USB_NETWORK"
)

// runtimeConfig mirrors the subset of options that pkg/app.Config used to
// take. It is captured here so the cobra wiring keeps working while pkg/app
// is being rewritten.
type runtimeConfig struct {
	BaseURL                string
	Password               string
	RPCTimeout             time.Duration
	ExperimentalUSBNetwork bool
}

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
	cfg := runtimeConfig{}
	logLevel := ""
	passwordFromStdin := false
	passwordEnv := ""

	rootCmd := &cobra.Command{
		Use:   "jetkvm-desktop [base-url-or-host]",
		Short: "Desktop JetKVM client (Gio rewrite, work in progress)",
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

			go func() {
				if err := runWindow(cfg); err != nil {
					log.Fatalf("window: %v", err)
				}
				os.Exit(0)
			}()
			app.Main()
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

func runWindow(cfg runtimeConfig) error {
	w := new(app.Window)
	w.Option(
		app.Title("JetKVM Desktop"),
		app.Size(unit.Dp(1024), unit.Dp(720)),
	)

	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(nil))

	var ops op.Ops
	var (
		paintCount int
		lastReport = time.Now()
		lastFPS    float64
	)

	for {
		ev := w.Event()
		switch e := ev.(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			ops.Reset()
			gtx := app.NewContext(&ops, e)

			label := material.H4(th, "JetKVM Desktop \u2013 Gio rewrite")
			label.Alignment = text.Middle

			sub := material.Body1(th, gioSkeletonStatus(cfg, lastFPS))
			sub.Alignment = text.Middle

			layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{
					Axis:      layout.Vertical,
					Alignment: layout.Middle,
				}.Layout(gtx,
					layout.Rigid(label.Layout),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(sub.Layout),
				)
			})

			e.Frame(gtx.Ops)

			paintCount++
			if elapsed := time.Since(lastReport); elapsed >= time.Second {
				lastFPS = float64(paintCount) / elapsed.Seconds()
				paintCount = 0
				lastReport = time.Now()
				log.Printf("[gio] %.1f fps", lastFPS)
			}

			// Bench mode: drive a continuous redraw to measure idle paint
			// cost. In production we let Gio repaint only when something
			// invalidates the frame (input events, new video frame, etc.).
			if envEnabled("JETKVM_DESKTOP_BENCH", os.Getenv) {
				w.Invalidate()
			}
		case app.ConfigEvent:
			// Some WMs (notably lightweight X11 setups without a compositor)
			// don't deliver an Expose event after the initial map, so Gio
			// can't synthesise the first FrameEvent on its own. Kick the
			// loop from a goroutine so the event consumer is parked when
			// Invalidate fires and Gio's internal mayInvalidate latch is
			// armed.
			go func() {
				time.Sleep(50 * time.Millisecond)
				w.Invalidate()
			}()
		}
	}
}

func gioSkeletonStatus(cfg runtimeConfig, fps float64) string {
	parts := []string{"backend not wired yet"}
	if cfg.BaseURL != "" {
		parts = append(parts, fmt.Sprintf("target=%s", cfg.BaseURL))
	}
	if fps > 0 {
		parts = append(parts, fmt.Sprintf("%.1f fps", fps))
	}
	return strings.Join(parts, "  \u2022  ")
}
