package app

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/lkarlslund/jetkvm-desktop/pkg/discovery"
	"github.com/lkarlslund/jetkvm-desktop/pkg/ui"
)

func (a *App) syncDiscovery() {
	if a.discovery == nil {
		return
	}
	for {
		select {
		case device := <-a.discovery.Updates():
			a.addDiscoveredDevice(device)
		default:
			return
		}
	}
}

func (a *App) addDiscoveredDevice(device discovery.Device) {
	for i := range a.discovered {
		if a.discovered[i].BaseURL == device.BaseURL {
			a.discovered[i] = device
			a.sortDiscovered()
			return
		}
	}
	a.discovered = append(a.discovered, device)
	a.sortDiscovered()
}

func (a *App) sortDiscovered() {
	slices.SortFunc(a.discovered, func(a, b discovery.Device) int {
		if a.Name != b.Name {
			return strings.Compare(a.Name, b.Name)
		}
		return strings.Compare(a.BaseURL, b.BaseURL)
	})
}

func (a *App) syncLauncherInput() {
	switch a.launcherMode {
	case launcherModeBrowse:
		if a.textInput.FieldID != "launcher_focus_input" {
			a.textInput.Sync(&ui.TextInputBinding{
				ID:       "launcher_focus_input",
				Value:    a.launcherInput,
				TextSize: 15,
			})
		}
	case launcherModePassword:
		if a.textInput.FieldID != "launcher_focus_password" {
			a.textInput.Sync(&ui.TextInputBinding{
				ID:           "launcher_focus_password",
				Value:        a.launcherPassword,
				DisplayValue: strings.Repeat("*", len([]rune(a.launcherPassword))),
				TextSize:     15,
			})
		}
	}
	a.syncFocusedTextInput()
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		if a.launcherMode == launcherModePassword {
			a.connectFromLauncher(a.pendingTarget)
		} else {
			a.saveAsRecent = true
			a.connectFromLauncher(a.launcherInput)
		}
		return
	}
}

func (a *App) drawLauncher(screen *ebiten.Image) {
	screen.Fill(a.currentTheme().Background)

	if a.launcherMode == launcherModePassword {
		a.drawPasswordPrompt(screen)
		return
	}

	a.drawUIRoot(screen, &a.launcherRuntime, func(chromeButton) {}, launcherScreenElement{app: a})
}

func (a *App) drawPasswordPrompt(screen *ebiten.Image) {
	a.drawUIRoot(screen, &a.launcherRuntime, func(chromeButton) {}, launcherPasswordScreenElement{app: a})
}

type launcherScreenElement struct {
	app *App
}

func (launcherScreenElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e launcherScreenElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	validInput := strings.TrimSpace(e.app.launcherInput) != "" && isValidConnectHost(strings.TrimSpace(e.app.launcherInput))
	hasRecent := len(e.app.prefs.RecentConnections) > 0
	discoveryMaxH := 520.0
	if hasRecent {
		discoveryMaxH = 280
	}
	children := []ui.Child{
		ui.Fixed(ui.Label{Text: "JetKVM", Size: 30, Color: ctx.Theme.Title}),
		ui.Fixed(ui.Spacer{H: 12}),
		ui.Fixed(ui.Label{Text: "Available devices on your local network", Size: 15, Color: ctx.Theme.Muted}),
		ui.Fixed(ui.Spacer{H: 28}),
		ui.Flex(ui.Constrained{
			MinH: 120,
			MaxH: discoveryMaxH,
			Child: ui.Panel{
				Fill:   ctx.Theme.PanelFill,
				Stroke: ctx.Theme.PanelStroke,
				Insets: ui.UniformInsets(18),
				Child:  launcherListElement(e),
			},
		}, 1),
	}
	if hasRecent {
		children = append(children,
			ui.Fixed(ui.Spacer{H: 18}),
			ui.Fixed(ui.Label{Text: "Recently connected", Size: 15, Color: ctx.Theme.Muted}),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(launcherRecentListElement{app: e.app}),
		)
	}
	children = append(children,
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(ui.Column{
			Children: []ui.Child{
				ui.Fixed(ui.Label{Text: "Connect by host, DNS name, or IP", Size: 13, Color: ctx.Theme.Muted}),
				ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(ui.Row{
					Children: []ui.Child{
						ui.Flex(launcherInputElement(e), 1),
						ui.Fixed(ui.Button{Label: "Connect", Enabled: validInput, OnClick: func() {
							e.app.saveAsRecent = true
							e.app.connectFromLauncher(e.app.launcherInput)
						}}),
					},
					Spacing: 12,
				}),
			},
		}))
	if e.app.launcherError != "" {
		children = append(children,
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(ui.Paragraph{Text: e.app.launcherError, Size: 12, Color: ctx.Theme.Error}),
		)
	} else if strings.TrimSpace(e.app.launcherInput) != "" && !validInput {
		children = append(children,
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(ui.Paragraph{Text: "Enter a valid hostname or IP address.", Size: 12, Color: ctx.Theme.Error}),
		)
	}
	ui.Inset{
		Insets: ui.Insets{Top: 42, Right: 48, Bottom: 44, Left: 48},
		Child: ui.Constrained{
			MaxW:  1380,
			Child: ui.Column{Children: children},
		},
	}.Draw(ctx, bounds)
}

type launcherPasswordScreenElement struct {
	app *App
}

func (launcherPasswordScreenElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e launcherPasswordScreenElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	targetLabel := e.app.pendingTarget
	if targetLabel == "" {
		targetLabel = e.app.launcherInput
	}
	ui.Inset{
		Insets: ui.UniformInsets(48),
		Child: ui.Align{
			Horizontal: ui.AlignCenter,
			Vertical:   ui.AlignCenter,
			Child: ui.Column{
				Children: []ui.Child{
					ui.Fixed(ui.Label{Text: "JetKVM", Size: 30, Color: ctx.Theme.Title}),
					ui.Fixed(ui.Spacer{H: 26}),
					ui.Fixed(ui.Constrained{
						MaxW: 620,
						Child: ui.Panel{
							Fill:   ctx.Theme.PanelFill,
							Stroke: ctx.Theme.PanelStroke,
							Insets: ui.UniformInsets(24),
							Child: launcherPasswordElement{
								app:         e.app,
								targetLabel: targetLabel,
							},
						},
					}),
				},
			},
		},
	}.Draw(ctx, bounds)
}

type launcherListElement struct {
	app *App
}

func (e launcherListElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e launcherListElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	if len(e.app.discovered) == 0 {
		ui.Column{
			Children: []ui.Child{
				ui.Fixed(ui.Label{Text: "Scanning local subnets for JetKVM devices...", Size: 16, Color: ctx.Theme.AccentText}),
				ui.Fixed(ui.Spacer{H: 12}),
				ui.Fixed(ui.Paragraph{
					Text:  "Devices will appear here as soon as they answer the JetKVM HTTP status endpoint.",
					Size:  13,
					Color: ctx.Theme.Muted,
				}),
			},
		}.Draw(ctx, bounds)
		return
	}
	children := make([]ui.Child, 0, len(e.app.discovered)*2+2)
	lastUpdated := ""
	for i, device := range e.app.discovered {
		if i >= 7 {
			break
		}
		if i > 0 {
			children = append(children, ui.Fixed(ui.Spacer{H: 8}))
		}
		children = append(children, ui.Fixed(launcherDevicePanelElement{app: e.app, device: device}))
		lastUpdated = fmt.Sprintf("Updated %s", humanDiscoveryAge(device.UpdatedAt))
	}
	if lastUpdated != "" {
		children = append(children, ui.Flex(ui.Spacer{}, 1), ui.Fixed(ui.Label{Text: lastUpdated, Size: 11, Color: ctx.Theme.DisabledText}))
	}
	ui.Column{Children: children}.Draw(ctx, bounds)
}

type launcherDevicePanelElement struct {
	app    *App
	device discovery.Device
}

func (e launcherDevicePanelElement) Measure(ctx *ui.Context, constraints ui.Constraints) ui.Size {
	return ui.Panel{
		Fill:   ctx.Theme.SectionFill,
		Stroke: ctx.Theme.ActiveStroke,
		Insets: ui.SymmetricInsets(16, 12),
		Child:  launcherDeviceRowElement{device: e.device},
	}.Measure(ctx, constraints)
}

func (e launcherDevicePanelElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Panel{
		Fill:   ctx.Theme.SectionFill,
		Stroke: ctx.Theme.ActiveStroke,
		Insets: ui.SymmetricInsets(16, 12),
		Child:  launcherDeviceRowElement{device: e.device},
	}.Draw(ctx, bounds)
	if ctx.Runtime != nil {
		baseURL := e.device.BaseURL
		ctx.Runtime.Register(ui.Control{
			ID:      "discover:" + baseURL,
			Rect:    bounds,
			Enabled: true,
			OnClick: func(ui.PointerEvent) {
				e.app.connectFromLauncher(baseURL)
			},
		})
	} else {
		ctx.AddHit("discover:"+e.device.BaseURL, bounds, true)
	}
}

type launcherDeviceRowElement struct {
	device discovery.Device
}

func (e launcherDeviceRowElement) Measure(ctx *ui.Context, constraints ui.Constraints) ui.Size {
	state := "Configured"
	if !e.device.IsSetup {
		state = "Needs setup"
	}
	return ui.Row{
		AlignY: ui.AlignCenter,
		Children: []ui.Child{
			ui.Flex(ui.Column{
				Children: []ui.Child{
					ui.Fixed(ui.Label{Text: e.device.Name, Size: 17}),
					ui.Fixed(ui.Spacer{H: 8}),
					ui.Fixed(ui.Label{Text: e.device.BaseURL, Size: 13}),
				},
			}, 1),
			ui.Fixed(ui.Label{Text: state, Size: 13}),
		},
	}.Measure(ctx, constraints)
}

func (e launcherDeviceRowElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	state := "Configured"
	if !e.device.IsSetup {
		state = "Needs setup"
	}
	ui.Row{
		AlignY: ui.AlignCenter,
		Children: []ui.Child{
			ui.Flex(ui.Column{
				Children: []ui.Child{
					ui.Fixed(ui.Label{Text: e.device.Name, Size: 17, Color: ctx.Theme.Title}),
					ui.Fixed(ui.Spacer{H: 8}),
					ui.Fixed(ui.Label{Text: e.device.BaseURL, Size: 13, Color: ctx.Theme.Muted}),
				},
			}, 1),
			ui.Fixed(ui.Label{Text: state, Size: 13, Color: ctx.Theme.AccentText}),
		},
	}.Draw(ctx, bounds)
}

type launcherInputElement struct {
	app *App
}

func (launcherInputElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: 40})
}

func (e launcherInputElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	e.app.decorateTextField(ui.TextField{
		ID:               "launcher_focus_input",
		Value:            e.app.launcherInput,
		Placeholder:      "jetkvm.local or 192.168.1.50",
		Enabled:          true,
		TextSize:         15,
		FillColor:        ctx.Theme.InputFill,
		StrokeColor:      ctx.Theme.InputStroke,
		FocusColor:       ctx.Theme.InputFocus,
		TextColor:        ctx.Theme.Body,
		PlaceholderColor: ctx.Theme.DisabledText,
		CaretColor:       ctx.Theme.AccentText,
	}).Draw(ctx, bounds)
}

type launcherPasswordElement struct {
	app         *App
	targetLabel string
}

func (e launcherPasswordElement) Measure(ctx *ui.Context, constraints ui.Constraints) ui.Size {
	return ui.Column{Children: e.children()}.Measure(ctx, constraints)
}

func (e launcherPasswordElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Column{Children: e.children()}.Draw(ctx, bounds)
}

func (e launcherPasswordElement) children() []ui.Child {
	passDisplay := strings.Repeat("*", len([]rune(e.app.launcherPassword)))
	children := []ui.Child{
		ui.Fixed(ui.Label{Text: "Password Required", Size: 24, Color: e.app.currentTheme().Title}),
		ui.Fixed(ui.Spacer{H: 12}),
		ui.Fixed(ui.Paragraph{Text: e.targetLabel, Size: 14, Color: e.app.currentTheme().Muted}),
		ui.Fixed(ui.Spacer{H: 22}),
		ui.Fixed(launcherPasswordFieldElement{app: e.app, passDisplay: passDisplay}),
		ui.Fixed(ui.Spacer{H: 18}),
	}
	if e.app.launcherError != "" {
		children = append(children,
			ui.Fixed(ui.Paragraph{Text: e.app.launcherError, Size: 12, Color: e.app.currentTheme().Error}),
			ui.Fixed(ui.Spacer{H: 12}),
		)
	}
	children = append(children, ui.Fixed(ui.Row{
		Children: []ui.Child{
			ui.Fixed(ui.Button{Label: "Back", Enabled: true, OnClick: func() {
				e.app.launcherMode = launcherModeBrowse
				e.app.launcherPassword = ""
				e.app.launcherError = ""
				if e.app.cfg.BaseURL != "" && e.app.ctrl != nil {
					e.app.ctrl.Stop()
					e.app.ctrl = nil
				}
			}}),
			ui.Flex(ui.Spacer{}, 1),
			ui.Fixed(ui.Button{Label: "Connect", Enabled: strings.TrimSpace(e.app.launcherPassword) != "", OnClick: func() {
				e.app.connectFromLauncher(e.app.pendingTarget)
			}}),
		},
		Spacing: 12,
	}))
	return children
}

type launcherPasswordFieldElement struct {
	app         *App
	passDisplay string
}

func (launcherPasswordFieldElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: 38})
}

func (e launcherPasswordFieldElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	e.app.decorateTextField(ui.TextField{
		ID:               "launcher_focus_password",
		DisplayValue:     e.passDisplay,
		Placeholder:      "Local password",
		Enabled:          true,
		TextSize:         15,
		FillColor:        ctx.Theme.InputFill,
		StrokeColor:      ctx.Theme.WarningStroke,
		FocusColor:       ctx.Theme.WarningStroke,
		TextColor:        ctx.Theme.Body,
		PlaceholderColor: ctx.Theme.DisabledText,
		CaretColor:       ctx.Theme.Error,
	}).Draw(ctx, bounds)
}

const maxVisibleRecentConnections = 3
const recentRowHeight = 36.0
const recentRowSpacing = 4.0

type launcherRecentListElement struct {
	app *App
}

func (e launcherRecentListElement) visibleCount() int {
	n := len(e.app.prefs.RecentConnections)
	if n > maxVisibleRecentConnections {
		n = maxVisibleRecentConnections
	}
	return n
}

func (e launcherRecentListElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	n := e.visibleCount()
	h := float64(n)*recentRowHeight + float64(n-1)*recentRowSpacing
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: h})
}

func (e launcherRecentListElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	n := e.visibleCount()
	y := bounds.Y
	for i := 0; i < n; i++ {
		rc := e.app.prefs.RecentConnections[i]
		rowBounds := ui.Rect{X: bounds.X, Y: y, W: bounds.W, H: recentRowHeight}

		ui.Panel{
			Fill:   ctx.Theme.SectionFill,
			Stroke: ctx.Theme.PanelStroke,
			Insets: ui.SymmetricInsets(12, 0),
			Child:  launcherRecentRowContentElement{rc: rc},
		}.Draw(ctx, rowBounds)

		removeW := 28.0
		connectRect := ui.Rect{X: rowBounds.X, Y: rowBounds.Y, W: rowBounds.W - removeW, H: rowBounds.H}
		removeRect := ui.Rect{X: rowBounds.X + rowBounds.W - removeW, Y: rowBounds.Y, W: removeW, H: rowBounds.H}

		url := rc.URL
		if ctx.Runtime != nil {
			ctx.Runtime.Register(ui.Control{
				ID: "recent:" + url, Rect: connectRect, Enabled: true,
				OnClick: func(ui.PointerEvent) { e.app.connectFromLauncher(url) },
			})
			ctx.Runtime.Register(ui.Control{
				ID: "recent_remove:" + url, Rect: removeRect, Enabled: true,
				OnClick: func(ui.PointerEvent) {
					e.app.prefs.removeRecentConnection(url)
					_ = savePreferences(e.app.prefs)
				},
			})
		} else {
			ctx.AddHit("recent:"+url, connectRect, true)
			ctx.AddHit("recent_remove:"+url, removeRect, true)
		}

		y += recentRowHeight + recentRowSpacing
	}
}

type launcherRecentRowContentElement struct {
	rc RecentConnection
}

func (launcherRecentRowContentElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: recentRowHeight})
}

func (e launcherRecentRowContentElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	label := e.rc.URL
	if e.rc.Name != "" {
		label = e.rc.Name
	}
	ui.Row{
		AlignY: ui.AlignCenter,
		Children: []ui.Child{
			ui.Flex(ui.Label{Text: label, Size: 14, Color: ctx.Theme.Title}, 1),
			ui.Fixed(ui.Label{Text: humanRecentAge(e.rc.ConnectedAt), Size: 11, Color: ctx.Theme.DisabledText}),
			ui.Fixed(ui.Spacer{W: 10}),
			ui.Fixed(ui.Label{Text: "\u00d7", Size: 16, Color: ctx.Theme.Muted}),
		},
	}.Draw(ctx, bounds)
}

func humanRecentAge(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	age := time.Since(at)
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return fmt.Sprintf("%dm ago", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(age.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(age.Hours()/24))
	}
}

func humanDiscoveryAge(at time.Time) string {
	if at.IsZero() {
		return "just now"
	}
	age := time.Since(at)
	switch {
	case age < time.Second:
		return "just now"
	case age < time.Minute:
		return fmt.Sprintf("%ds ago", int(age.Seconds()))
	default:
		return fmt.Sprintf("%dm ago", int(age.Minutes()))
	}
}
