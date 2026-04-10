package app

import (
	"fmt"
	"image/color"
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
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		value := a.launcherInput
		if a.launcherMode == launcherModePassword {
			value = a.launcherPassword
		}
		runes := []rune(value)
		if len(runes) > 0 {
			value = string(runes[:len(runes)-1])
			if a.launcherMode == launcherModeBrowse {
				a.launcherInput = value
			} else {
				a.launcherPassword = value
			}
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		if a.launcherMode == launcherModePassword {
			a.connectFromLauncher(a.pendingTarget)
		} else {
			a.connectFromLauncher(a.launcherInput)
		}
		return
	}
	for _, r := range ebiten.AppendInputChars(nil) {
		if r >= 32 && r != 127 {
			if a.launcherMode == launcherModeBrowse {
				a.launcherInput += string(r)
			} else {
				a.launcherPassword += string(r)
			}
		}
	}
}

func (a *App) drawLauncher(screen *ebiten.Image) {
	bounds := screen.Bounds()
	screen.Fill(color.RGBA{R: 11, G: 16, B: 24, A: 255})

	if a.launcherMode == launcherModePassword {
		a.drawPasswordPrompt(screen)
		return
	}

	a.launcherButtons = a.launcherButtons[:0]
	ctx := a.newUIContext(screen, func(btn chromeButton) {
		a.launcherButtons = append(a.launcherButtons, btn)
	})
	launcherScreenElement{app: a}.Draw(ctx, ui.Rect{W: float64(bounds.Dx()), H: float64(bounds.Dy())})
}

func (a *App) drawPasswordPrompt(screen *ebiten.Image) {
	bounds := screen.Bounds()
	a.launcherButtons = a.launcherButtons[:0]
	ctx := a.newUIContext(screen, func(btn chromeButton) {
		a.launcherButtons = append(a.launcherButtons, btn)
	})
	launcherPasswordScreenElement{app: a}.Draw(ctx, ui.Rect{W: float64(bounds.Dx()), H: float64(bounds.Dy())})
}

type launcherScreenElement struct {
	app *App
}

func (launcherScreenElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e launcherScreenElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	validInput := strings.TrimSpace(e.app.launcherInput) != "" && isValidConnectHost(strings.TrimSpace(e.app.launcherInput))
	children := []ui.Child{
		ui.Fixed(ui.Label{Text: "JetKVM", Size: 30, Color: color.RGBA{R: 241, G: 245, B: 249, A: 255}}),
		ui.Fixed(ui.Spacer{H: 12}),
		ui.Fixed(ui.Label{Text: "Available devices on your local network", Size: 15, Color: color.RGBA{R: 148, G: 163, B: 184, A: 255}}),
		ui.Fixed(ui.Spacer{H: 28}),
		ui.Fixed(ui.Constrained{
			MinH: 240,
			MaxH: 520,
			Child: ui.Panel{
				Fill:   color.RGBA{R: 15, G: 23, B: 34, A: 255},
				Stroke: color.RGBA{R: 71, G: 85, B: 105, A: 180},
				Insets: ui.UniformInsets(18),
				Child:  launcherListElement(e),
			},
		}),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(ui.Column{
			Children: []ui.Child{
				ui.Fixed(ui.Label{Text: "Connect by host, DNS name, or IP", Size: 13, Color: color.RGBA{R: 148, G: 163, B: 184, A: 255}}),
				ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(ui.Row{
					Children: []ui.Child{
						ui.Flex(launcherInputElement(e), 1),
						ui.Fixed(ui.Button{ID: "launcher_connect", Label: "Connect", Enabled: validInput}),
					},
					Spacing: 12,
				}),
			},
		}),
	}
	if e.app.launcherError != "" {
		children = append(children,
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(ui.Paragraph{Text: e.app.launcherError, Size: 12, Color: color.RGBA{R: 252, G: 165, B: 165, A: 255}}),
		)
	} else if strings.TrimSpace(e.app.launcherInput) != "" && !validInput {
		children = append(children,
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(ui.Paragraph{Text: "Enter a valid hostname or IP address.", Size: 12, Color: color.RGBA{R: 252, G: 165, B: 165, A: 255}}),
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
					ui.Fixed(ui.Label{Text: "JetKVM", Size: 30, Color: color.RGBA{R: 241, G: 245, B: 249, A: 255}}),
					ui.Fixed(ui.Spacer{H: 26}),
					ui.Fixed(ui.Constrained{
						MaxW: 620,
						Child: ui.Panel{
							Fill:   color.RGBA{R: 15, G: 23, B: 34, A: 255},
							Stroke: color.RGBA{R: 71, G: 85, B: 105, A: 180},
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
				ui.Fixed(ui.Label{Text: "Scanning local subnets for JetKVM devices...", Size: 16, Color: color.RGBA{R: 191, G: 219, B: 254, A: 255}}),
				ui.Fixed(ui.Spacer{H: 12}),
				ui.Fixed(ui.Paragraph{
					Text:  "Devices will appear here as soon as they answer the JetKVM HTTP status endpoint.",
					Size:  13,
					Color: color.RGBA{R: 148, G: 163, B: 184, A: 255},
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
		children = append(children, ui.Flex(ui.Spacer{}, 1), ui.Fixed(ui.Label{Text: lastUpdated, Size: 11, Color: color.RGBA{R: 100, G: 116, B: 139, A: 255}}))
	}
	ui.Column{Children: children}.Draw(ctx, bounds)
}

type launcherDevicePanelElement struct {
	app    *App
	device discovery.Device
}

func (launcherDevicePanelElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: 54})
}

func (e launcherDevicePanelElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Panel{
		Fill:   color.RGBA{R: 20, G: 31, B: 46, A: 255},
		Stroke: color.RGBA{R: 56, G: 189, B: 248, A: 80},
		Insets: ui.SymmetricInsets(16, 12),
		Child:  launcherDeviceRowElement{device: e.device},
	}.Draw(ctx, bounds)
	ctx.AddHit("discover:"+e.device.BaseURL, bounds, true)
}

type launcherDeviceRowElement struct {
	device discovery.Device
}

func (launcherDeviceRowElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: 54})
}

func (e launcherDeviceRowElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	state := "Configured"
	if !e.device.IsSetup {
		state = "Needs setup"
	}
	ui.Row{
		Children: []ui.Child{
			ui.Flex(ui.Align{
				Vertical: ui.AlignCenter,
				Child: ui.Column{
					Children: []ui.Child{
						ui.Fixed(ui.Label{Text: e.device.Name, Size: 17, Color: color.RGBA{R: 241, G: 245, B: 249, A: 255}}),
						ui.Fixed(ui.Spacer{H: 8}),
						ui.Fixed(ui.Label{Text: e.device.BaseURL, Size: 13, Color: color.RGBA{R: 148, G: 163, B: 184, A: 255}}),
					},
				},
			}, 1),
			ui.Fixed(ui.Align{
				Vertical: ui.AlignCenter,
				Child:    ui.Label{Text: state, Size: 13, Color: color.RGBA{R: 191, G: 219, B: 254, A: 255}},
			}),
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
	ui.TextField{
		Value:            e.app.launcherInput,
		Placeholder:      "jetkvm.local or 192.168.1.50",
		Focused:          true,
		Enabled:          true,
		TextSize:         15,
		FillColor:        color.RGBA{R: 15, G: 23, B: 34, A: 255},
		StrokeColor:      color.RGBA{R: 71, G: 85, B: 105, A: 180},
		FocusColor:       color.RGBA{R: 96, G: 165, B: 250, A: 180},
		TextColor:        color.RGBA{R: 241, G: 245, B: 249, A: 255},
		PlaceholderColor: color.RGBA{R: 100, G: 116, B: 139, A: 255},
		CaretColor:       color.RGBA{R: 191, G: 219, B: 254, A: 255},
	}.Draw(ctx, bounds)
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
		ui.Fixed(ui.Label{Text: "Password Required", Size: 24, Color: color.RGBA{R: 241, G: 245, B: 249, A: 255}}),
		ui.Fixed(ui.Spacer{H: 12}),
		ui.Fixed(ui.Paragraph{Text: e.targetLabel, Size: 14, Color: color.RGBA{R: 148, G: 163, B: 184, A: 255}}),
		ui.Fixed(ui.Spacer{H: 22}),
		ui.Fixed(launcherPasswordFieldElement{app: e.app, passDisplay: passDisplay}),
		ui.Fixed(ui.Spacer{H: 18}),
	}
	if e.app.launcherError != "" {
		children = append(children,
			ui.Fixed(ui.Paragraph{Text: e.app.launcherError, Size: 12, Color: color.RGBA{R: 252, G: 165, B: 165, A: 255}}),
			ui.Fixed(ui.Spacer{H: 12}),
		)
	}
	children = append(children, ui.Fixed(ui.Row{
		Children: []ui.Child{
			ui.Fixed(ui.Button{ID: "launcher_back", Label: "Back", Enabled: true}),
			ui.Flex(ui.Spacer{}, 1),
			ui.Fixed(ui.Button{ID: "launcher_retry_password", Label: "Connect", Enabled: strings.TrimSpace(e.app.launcherPassword) != ""}),
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
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: 72})
}

func (e launcherPasswordFieldElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.TextField{
		DisplayValue:     e.passDisplay,
		Placeholder:      "Local password",
		Focused:          true,
		Enabled:          true,
		TextSize:         15,
		FillColor:        color.RGBA{R: 11, G: 16, B: 24, A: 255},
		StrokeColor:      color.RGBA{R: 127, G: 29, B: 29, A: 180},
		FocusColor:       color.RGBA{R: 127, G: 29, B: 29, A: 180},
		TextColor:        color.RGBA{R: 241, G: 245, B: 249, A: 255},
		PlaceholderColor: color.RGBA{R: 100, G: 116, B: 139, A: 255},
		CaretColor:       color.RGBA{R: 248, G: 113, B: 113, A: 255},
	}.Draw(ctx, bounds)
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
