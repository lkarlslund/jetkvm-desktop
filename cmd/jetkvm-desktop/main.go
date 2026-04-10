package main

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/spf13/cobra"

	"github.com/lkarlslund/jetkvm-desktop/pkg/app"
)

const (
	defaultWindowWidth  = 1280
	defaultWindowHeight = 720
	targetWindowWidth   = 1920
	targetWindowHeight  = 1080
	usableMonitorFactor = 0.9
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

			windowWidth, windowHeight := initialWindowSize()
			ebiten.SetWindowSize(windowWidth, windowHeight)
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

func initialWindowSize() (int, int) {
	monitor := ebiten.Monitor()
	if monitor == nil {
		return defaultWindowWidth, defaultWindowHeight
	}
	return initialWindowSizeForMonitor(monitor.Size())
}

func initialWindowSizeForMonitor(monitorWidth, monitorHeight int) (int, int) {
	if monitorWidth <= 0 || monitorHeight <= 0 {
		return defaultWindowWidth, defaultWindowHeight
	}

	usableWidth := int(math.Floor(float64(monitorWidth) * usableMonitorFactor))
	usableHeight := int(math.Floor(float64(monitorHeight) * usableMonitorFactor))
	if usableWidth <= 0 || usableHeight <= 0 {
		return defaultWindowWidth, defaultWindowHeight
	}
	if usableWidth >= targetWindowWidth && usableHeight >= targetWindowHeight {
		return targetWindowWidth, targetWindowHeight
	}

	defaultScale := float64(defaultWindowWidth) / float64(targetWindowWidth)
	scale := min(
		float64(usableWidth)/float64(targetWindowWidth),
		float64(usableHeight)/float64(targetWindowHeight),
	)
	if usableWidth >= defaultWindowWidth && usableHeight >= defaultWindowHeight && scale < defaultScale {
		scale = defaultScale
	}
	if scale <= 0 {
		return defaultWindowWidth, defaultWindowHeight
	}
	return scaleWindow(targetWindowWidth, targetWindowHeight, scale)
}

func scaleWindow(baseWidth, baseHeight int, scale float64) (int, int) {
	width := int(math.Floor(float64(baseWidth) * scale))
	if width < 1 {
		width = 1
	}
	height := width * baseHeight / baseWidth
	if height < 1 {
		height = 1
	}
	return width, height
}
