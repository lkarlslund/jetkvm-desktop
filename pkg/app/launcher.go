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

	listX := 48.0
	listY := 102.0
	listW := float64(bounds.Dx()) - 96
	listH := float64(bounds.Dy()) - 214
	a.launcherButtons = a.launcherButtons[:0]
	inputY := float64(bounds.Dy()) - 84
	validInput := strings.TrimSpace(a.launcherInput) != "" && isValidConnectHost(strings.TrimSpace(a.launcherInput))
	ctx := a.newUIContext(screen, func(btn chromeButton) {
		a.launcherButtons = append(a.launcherButtons, btn)
	})
	ui.Column{
		Children: []ui.Child{
			ui.Fixed(ui.Label{Text: "JetKVM", Size: 30, Color: color.RGBA{R: 241, G: 245, B: 249, A: 255}}),
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(ui.Label{Text: "Available devices on your local network", Size: 15, Color: color.RGBA{R: 148, G: 163, B: 184, A: 255}}),
		},
	}.Draw(ctx, ui.Rect{X: 48, Y: 42, W: listW, H: 42})
	ui.Panel{
		Fill:   color.RGBA{R: 15, G: 23, B: 34, A: 255},
		Stroke: color.RGBA{R: 71, G: 85, B: 105, A: 180},
		Insets: ui.UniformInsets(18),
		Child:  launcherListElement{app: a},
	}.Draw(ctx, ui.Rect{X: listX, Y: listY, W: listW, H: listH})
	ui.Column{
		Children: []ui.Child{
			ui.Fixed(ui.Label{Text: "Connect by host, DNS name, or IP", Size: 13, Color: color.RGBA{R: 148, G: 163, B: 184, A: 255}}),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Row{
				Children: []ui.Child{
					ui.Flex(launcherInputTextElement(launcherInputElement{app: a}), 1),
					ui.Fixed(ui.Button{ID: "launcher_connect", Label: "Connect", Enabled: validInput}),
				},
				Spacing: 12,
			}),
		},
	}.Draw(ctx, ui.Rect{X: 48, Y: inputY - 18, W: listW, H: 66})
	if a.launcherError != "" {
		ui.Paragraph{
			Text:  a.launcherError,
			Size:  12,
			Color: color.RGBA{R: 252, G: 165, B: 165, A: 255},
		}.Draw(ctx, ui.Rect{X: 48, Y: inputY + 52, W: listW, H: 40})
	} else if strings.TrimSpace(a.launcherInput) != "" && !validInput {
		ui.Paragraph{
			Text:  "Enter a valid hostname or IP address.",
			Size:  12,
			Color: color.RGBA{R: 252, G: 165, B: 165, A: 255},
		}.Draw(ctx, ui.Rect{X: 48, Y: inputY + 52, W: listW, H: 40})
	}
}

func (a *App) drawPasswordPrompt(screen *ebiten.Image) {
	bounds := screen.Bounds()
	panelW := min(float64(bounds.Dx())-96, 620)
	errorHeight := 0.0
	if a.launcherError != "" {
		errorHeight = wrappedTextHeight(a.launcherError, panelW-48, 12) + 18
	}
	panelH := 230.0 + errorHeight
	panelX := (float64(bounds.Dx()) - panelW) / 2
	panelY := (float64(bounds.Dy()) - panelH) / 2

	targetLabel := a.pendingTarget
	if targetLabel == "" {
		targetLabel = a.launcherInput
	}
	a.launcherButtons = a.launcherButtons[:0]
	ctx := a.newUIContext(screen, func(btn chromeButton) {
		a.launcherButtons = append(a.launcherButtons, btn)
	})
	ui.Label{Text: "JetKVM", Size: 30, Color: color.RGBA{R: 241, G: 245, B: 249, A: 255}}.
		Draw(ctx, ui.Rect{X: panelX, Y: panelY - 56, W: panelW, H: 30})
	ui.Panel{
		Fill:   color.RGBA{R: 15, G: 23, B: 34, A: 255},
		Stroke: color.RGBA{R: 71, G: 85, B: 105, A: 180},
		Insets: ui.UniformInsets(24),
		Child: launcherPasswordElement{
			app:         a,
			targetLabel: targetLabel,
		},
	}.Draw(ctx, ui.Rect{X: panelX, Y: panelY, W: panelW, H: panelH})
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
	rowY := bounds.Y
	lastUpdated := ""
	for i, device := range e.app.discovered {
		if rowY+54 > bounds.Bottom()-18 {
			break
		}
		rowBounds := ui.Rect{X: bounds.X, Y: rowY, W: bounds.W, H: 54}
		ui.Panel{
			Fill:   color.RGBA{R: 20, G: 31, B: 46, A: 255},
			Stroke: color.RGBA{R: 56, G: 189, B: 248, A: 80},
			Insets: ui.SymmetricInsets(16, 12),
			Child:  launcherDeviceRowElement{device: device},
		}.Draw(ctx, rowBounds)
		ctx.AddHit("discover:"+device.BaseURL, rowBounds, true)
		rowY += 62
		if i == len(e.app.discovered)-1 {
			lastUpdated = fmt.Sprintf("Updated %s", humanDiscoveryAge(device.UpdatedAt))
		}
	}
	if lastUpdated != "" {
		ui.Label{Text: lastUpdated, Size: 11, Color: color.RGBA{R: 100, G: 116, B: 139, A: 255}}.
			Draw(ctx, ui.Rect{X: bounds.X + 4, Y: bounds.Bottom() - 12, W: bounds.W, H: 12})
	}
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
			ui.Flex(ui.Column{
				Children: []ui.Child{
					ui.Fixed(ui.Label{Text: e.device.Name, Size: 17, Color: color.RGBA{R: 241, G: 245, B: 249, A: 255}}),
					ui.Fixed(ui.Spacer{H: 8}),
					ui.Fixed(ui.Label{Text: e.device.BaseURL, Size: 13, Color: color.RGBA{R: 148, G: 163, B: 184, A: 255}}),
				},
			}, 1),
			ui.Fixed(ui.Label{Text: state, Size: 13, Color: color.RGBA{R: 191, G: 219, B: 254, A: 255}}),
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
	ui.Panel{
		Fill:   color.RGBA{R: 15, G: 23, B: 34, A: 255},
		Stroke: color.RGBA{R: 71, G: 85, B: 105, A: 180},
		Insets: ui.SymmetricInsets(14, 12),
		Child:  launcherInputTextElement(e),
	}.Draw(ctx, bounds)
}

type launcherInputTextElement struct {
	app *App
}

func (launcherInputTextElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e launcherInputTextElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	text := e.app.launcherInput
	textColor := color.RGBA{R: 241, G: 245, B: 249, A: 255}
	if text == "" {
		text = "jetkvm.local or 192.168.1.50"
		textColor = color.RGBA{R: 100, G: 116, B: 139, A: 255}
	}
	ui.Label{Text: text, Size: 15, Color: textColor}.Draw(ctx, bounds)
	if strings.TrimSpace(e.app.launcherInput) != "" && time.Now().UnixMilli()/500%2 == 0 {
		textW, _ := ctx.MeasureText(text, 15)
		ctx.FillRect(ui.Rect{X: bounds.X + textW + 2, Y: bounds.Y - 4, W: 2, H: 20}, color.RGBA{R: 191, G: 219, B: 254, A: 255})
	}
}

type launcherPasswordElement struct {
	app         *App
	targetLabel string
}

func (e launcherPasswordElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e launcherPasswordElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	passDisplay := strings.Repeat("*", len([]rune(e.app.launcherPassword)))
	children := []ui.Child{
		ui.Fixed(ui.Label{Text: "Password Required", Size: 24, Color: color.RGBA{R: 241, G: 245, B: 249, A: 255}}),
		ui.Fixed(ui.Spacer{H: 12}),
		ui.Fixed(ui.Paragraph{Text: e.targetLabel, Size: 14, Color: color.RGBA{R: 148, G: 163, B: 184, A: 255}}),
		ui.Fixed(ui.Spacer{H: 22}),
		ui.Fixed(ui.Panel{
			Fill:   color.RGBA{R: 11, G: 16, B: 24, A: 255},
			Stroke: color.RGBA{R: 127, G: 29, B: 29, A: 180},
			Insets: ui.SymmetricInsets(14, 14),
			Child:  launcherPasswordFieldElement{app: e.app, passDisplay: passDisplay},
		}),
		ui.Flex(ui.Spacer{}, 1),
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
	ui.Column{Children: children}.Draw(ctx, bounds)
}

type launcherPasswordFieldElement struct {
	app         *App
	passDisplay string
}

func (launcherPasswordFieldElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: 44})
}

func (e launcherPasswordFieldElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	if e.passDisplay == "" {
		ui.Label{Text: "Local password", Size: 15, Color: color.RGBA{R: 100, G: 116, B: 139, A: 255}}.Draw(ctx, bounds)
		return
	}
	ui.Label{Text: e.passDisplay, Size: 15, Color: color.RGBA{R: 241, G: 245, B: 249, A: 255}}.Draw(ctx, bounds)
	if time.Now().UnixMilli()/500%2 == 0 {
		textW, _ := ctx.MeasureText(e.passDisplay, 15)
		ctx.FillRect(ui.Rect{X: bounds.X + textW + 2, Y: bounds.Y - 4, W: 2, H: 22}, color.RGBA{R: 248, G: 113, B: 113, A: 255})
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
