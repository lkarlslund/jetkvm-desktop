package app

import (
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	DefaultWindowWidth  = 1280
	DefaultWindowHeight = 720
	BrowseWindowWidth   = 480
	BrowseWindowHeight  = 640
	targetWindowWidth   = 1920
	targetWindowHeight  = 1080
	usableMonitorFactor = 0.9
)

func InitialWindowSize(browseMode bool) (int, int) {
	monitor := ebiten.Monitor()
	if monitor == nil {
		return baseWindowSize(browseMode)
	}
	monitorWidth, monitorHeight := monitor.Size()
	return InitialWindowSizeForMonitor(monitorWidth, monitorHeight, browseMode)
}

func InitialWindowSizeForMonitor(monitorWidth, monitorHeight int, browseMode bool) (int, int) {
	baseWidth, baseHeight := baseWindowSize(browseMode)
	if browseMode {
		return baseWidth, baseHeight
	}
	if monitorWidth <= 0 || monitorHeight <= 0 {
		return baseWidth, baseHeight
	}

	usableWidth := int(math.Floor(float64(monitorWidth) * usableMonitorFactor))
	usableHeight := int(math.Floor(float64(monitorHeight) * usableMonitorFactor))
	if usableWidth <= 0 || usableHeight <= 0 {
		return baseWidth, baseHeight
	}
	if usableWidth >= targetWindowWidth && usableHeight >= targetWindowHeight {
		if browseMode {
			return targetWindowWidth, BrowseWindowHeight
		}
		return targetWindowWidth, targetWindowHeight
	}

	defaultScale := float64(baseWidth) / float64(targetWindowWidth)
	scale := min(
		float64(usableWidth)/float64(targetWindowWidth),
		float64(usableHeight)/float64(targetWindowHeight),
	)
	if usableWidth >= baseWidth && usableHeight >= baseHeight && scale < defaultScale {
		scale = defaultScale
	}
	if scale <= 0 {
		return baseWidth, baseHeight
	}
	width, height := scaleWindow(targetWindowWidth, targetWindowHeight, scale)
	if browseMode && height > BrowseWindowHeight {
		height = BrowseWindowHeight
	}
	return width, height
}

func baseWindowSize(browseMode bool) (int, int) {
	if browseMode {
		return BrowseWindowWidth, BrowseWindowHeight
	}
	return DefaultWindowWidth, DefaultWindowHeight
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

func (a *App) maybeExpandBrowseWindow() {
	if !a.launcherOpen || strings.TrimSpace(a.cfg.BaseURL) != "" {
		return
	}
	width, height := InitialWindowSize(false)
	ebiten.SetWindowSize(width, height)
}

func (a *App) applyConnectWindowMode() {
	switch a.prefs.ConnectWindowMode {
	case connectWindowFullscreen:
		if !ebiten.IsFullscreen() {
			ebiten.SetFullscreen(true)
		}
	case connectWindowPixelRatio:
		ebiten.SetWindowSize(targetWindowWidth, targetWindowHeight)
	case connectWindowMaximize:
		if !ebiten.IsFullscreen() && !ebiten.IsWindowMaximized() {
			ebiten.MaximizeWindow()
		}
	case connectWindowUnchanged:
		// leave the window as-is
	}
}
