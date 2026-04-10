package app

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/lkarlslund/jetkvm-desktop/pkg/discovery"
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

	drawText(screen, "JetKVM", 48, 42, 30, color.RGBA{R: 241, G: 245, B: 249, A: 255})
	drawText(screen, "Available devices on your local network", 48, 72, 15, color.RGBA{R: 148, G: 163, B: 184, A: 255})

	listX := 48.0
	listY := 102.0
	listW := float64(bounds.Dx()) - 96
	listH := float64(bounds.Dy()) - 214

	vector.FillRect(screen, float32(listX), float32(listY), float32(listW), float32(listH), color.RGBA{R: 15, G: 23, B: 34, A: 255}, false)
	vector.StrokeRect(screen, float32(listX), float32(listY), float32(listW), float32(listH), 1, color.RGBA{R: 71, G: 85, B: 105, A: 180}, false)

	a.launcherButtons = a.launcherButtons[:0]
	if len(a.discovered) == 0 {
		drawText(screen, "Scanning local subnets for JetKVM devices...", listX+24, listY+34, 16, color.RGBA{R: 191, G: 219, B: 254, A: 255})
		drawWrappedText(screen, "Devices will appear here as soon as they answer the JetKVM HTTP status endpoint.", listX+24, listY+64, listW-48, 13, color.RGBA{R: 148, G: 163, B: 184, A: 255})
	} else {
		rowY := listY + 18
		for i, device := range a.discovered {
			if rowY+66 > listY+listH-12 {
				break
			}
			btn := chromeButton{
				id:      "discover:" + device.BaseURL,
				enabled: true,
				rect:    rect{x: listX + 14, y: rowY, w: listW - 28, h: 54},
			}
			a.launcherButtons = append(a.launcherButtons, btn)
			fill := color.RGBA{R: 20, G: 31, B: 46, A: 255}
			stroke := color.RGBA{R: 56, G: 189, B: 248, A: 80}
			vector.FillRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), fill, false)
			vector.StrokeRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), 1, stroke, false)
			drawText(screen, device.Name, btn.rect.x+16, btn.rect.y+12, 17, color.RGBA{R: 241, G: 245, B: 249, A: 255})
			drawText(screen, device.BaseURL, btn.rect.x+16, btn.rect.y+34, 13, color.RGBA{R: 148, G: 163, B: 184, A: 255})
			state := "Configured"
			if !device.IsSetup {
				state = "Needs setup"
			}
			drawText(screen, state, btn.rect.x+btn.rect.w-98, btn.rect.y+22, 13, color.RGBA{R: 191, G: 219, B: 254, A: 255})
			rowY += 62
			if i == len(a.discovered)-1 {
				drawText(screen, fmt.Sprintf("Updated %s", humanDiscoveryAge(device.UpdatedAt)), listX+22, listY+listH-20, 11, color.RGBA{R: 100, G: 116, B: 139, A: 255})
			}
		}
	}

	inputY := float64(bounds.Dy()) - 84
	drawText(screen, "Connect by host, DNS name, or IP", 48, inputY-18, 13, color.RGBA{R: 148, G: 163, B: 184, A: 255})
	vector.FillRect(screen, 48, float32(inputY), float32(listW-140), 40, color.RGBA{R: 15, G: 23, B: 34, A: 255}, false)
	vector.StrokeRect(screen, 48, float32(inputY), float32(listW-140), 40, 1, color.RGBA{R: 71, G: 85, B: 105, A: 180}, false)
	validInput := strings.TrimSpace(a.launcherInput) != "" && isValidConnectHost(strings.TrimSpace(a.launcherInput))
	inputText := a.launcherInput
	if inputText == "" {
		inputText = "jetkvm.local or 192.168.1.50"
		drawText(screen, inputText, 62, inputY+12, 15, color.RGBA{R: 100, G: 116, B: 139, A: 255})
	} else {
		drawText(screen, inputText, 62, inputY+12, 15, color.RGBA{R: 241, G: 245, B: 249, A: 255})
		if time.Now().UnixMilli()/500%2 == 0 {
			textW, _ := measureText(inputText, 15)
			vector.FillRect(screen, float32(64+textW), float32(inputY+8), 2, 20, color.RGBA{R: 191, G: 219, B: 254, A: 255}, false)
		}
	}

	connectBtn := chromeButton{
		id:      "launcher_connect",
		enabled: validInput,
		rect:    rect{x: 48 + listW - 128, y: inputY, w: 128, h: 40},
	}
	a.launcherButtons = append(a.launcherButtons, connectBtn)
	fill := color.RGBA{R: 37, G: 99, B: 235, A: 255}
	stroke := color.RGBA{R: 147, G: 197, B: 253, A: 180}
	textClr := color.RGBA{R: 239, G: 246, B: 255, A: 255}
	if !connectBtn.enabled {
		fill = color.RGBA{R: 30, G: 41, B: 59, A: 255}
		stroke = color.RGBA{R: 71, G: 85, B: 105, A: 160}
		textClr = color.RGBA{R: 148, G: 163, B: 184, A: 255}
	}
	vector.FillRect(screen, float32(connectBtn.rect.x), float32(connectBtn.rect.y), float32(connectBtn.rect.w), float32(connectBtn.rect.h), fill, false)
	vector.StrokeRect(screen, float32(connectBtn.rect.x), float32(connectBtn.rect.y), float32(connectBtn.rect.w), float32(connectBtn.rect.h), 1, stroke, false)
	drawText(screen, "Connect", connectBtn.rect.x+28, connectBtn.rect.y+12, 15, textClr)

	if a.launcherError != "" {
		drawWrappedText(screen, a.launcherError, 48, inputY+52, listW, 12, color.RGBA{R: 252, G: 165, B: 165, A: 255})
	} else if strings.TrimSpace(a.launcherInput) != "" && !validInput {
		drawWrappedText(screen, "Enter a valid hostname or IP address.", 48, inputY+52, listW, 12, color.RGBA{R: 252, G: 165, B: 165, A: 255})
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

	drawText(screen, "JetKVM", panelX, panelY-56, 30, color.RGBA{R: 241, G: 245, B: 249, A: 255})
	vector.FillRect(screen, float32(panelX), float32(panelY), float32(panelW), float32(panelH), color.RGBA{R: 15, G: 23, B: 34, A: 255}, false)
	vector.StrokeRect(screen, float32(panelX), float32(panelY), float32(panelW), float32(panelH), 1, color.RGBA{R: 71, G: 85, B: 105, A: 180}, false)

	targetLabel := a.pendingTarget
	if targetLabel == "" {
		targetLabel = a.launcherInput
	}
	drawText(screen, "Password Required", panelX+24, panelY+26, 24, color.RGBA{R: 241, G: 245, B: 249, A: 255})
	drawWrappedText(screen, targetLabel, panelX+24, panelY+58, panelW-48, 14, color.RGBA{R: 148, G: 163, B: 184, A: 255})

	fieldY := panelY + 112
	vector.FillRect(screen, float32(panelX+24), float32(fieldY), float32(panelW-48), 44, color.RGBA{R: 11, G: 16, B: 24, A: 255}, false)
	vector.StrokeRect(screen, float32(panelX+24), float32(fieldY), float32(panelW-48), 44, 1, color.RGBA{R: 127, G: 29, B: 29, A: 180}, false)
	passDisplay := strings.Repeat("*", len([]rune(a.launcherPassword)))
	if passDisplay == "" {
		drawText(screen, "Local password", panelX+38, fieldY+14, 15, color.RGBA{R: 100, G: 116, B: 139, A: 255})
	} else {
		drawText(screen, passDisplay, panelX+38, fieldY+14, 15, color.RGBA{R: 241, G: 245, B: 249, A: 255})
	}
	if time.Now().UnixMilli()/500%2 == 0 {
		textW, _ := measureText(passDisplay, 15)
		vector.FillRect(screen, float32(panelX+40+textW), float32(fieldY+10), 2, 22, color.RGBA{R: 248, G: 113, B: 113, A: 255}, false)
	}

	a.launcherButtons = a.launcherButtons[:0]
	buttonY := panelY + panelH - 56
	backBtn := chromeButton{
		id:      "launcher_back",
		enabled: true,
		rect:    rect{x: panelX + 24, y: buttonY, w: 110, h: 40},
	}
	connectBtn := chromeButton{
		id:      "launcher_retry_password",
		enabled: strings.TrimSpace(a.launcherPassword) != "",
		rect:    rect{x: panelX + panelW - 152, y: buttonY, w: 128, h: 40},
	}
	a.launcherButtons = append(a.launcherButtons, backBtn, connectBtn)

	vector.FillRect(screen, float32(backBtn.rect.x), float32(backBtn.rect.y), float32(backBtn.rect.w), float32(backBtn.rect.h), color.RGBA{R: 30, G: 41, B: 59, A: 255}, false)
	vector.StrokeRect(screen, float32(backBtn.rect.x), float32(backBtn.rect.y), float32(backBtn.rect.w), float32(backBtn.rect.h), 1, color.RGBA{R: 71, G: 85, B: 105, A: 160}, false)
	drawText(screen, "Back", backBtn.rect.x+34, backBtn.rect.y+12, 15, color.RGBA{R: 226, G: 232, B: 240, A: 255})

	fill := color.RGBA{R: 37, G: 99, B: 235, A: 255}
	stroke := color.RGBA{R: 147, G: 197, B: 253, A: 180}
	textClr := color.RGBA{R: 239, G: 246, B: 255, A: 255}
	if !connectBtn.enabled {
		fill = color.RGBA{R: 30, G: 41, B: 59, A: 255}
		stroke = color.RGBA{R: 71, G: 85, B: 105, A: 160}
		textClr = color.RGBA{R: 148, G: 163, B: 184, A: 255}
	}
	vector.FillRect(screen, float32(connectBtn.rect.x), float32(connectBtn.rect.y), float32(connectBtn.rect.w), float32(connectBtn.rect.h), fill, false)
	vector.StrokeRect(screen, float32(connectBtn.rect.x), float32(connectBtn.rect.y), float32(connectBtn.rect.w), float32(connectBtn.rect.h), 1, stroke, false)
	drawText(screen, "Connect", connectBtn.rect.x+28, connectBtn.rect.y+12, 15, textClr)

	if a.launcherError != "" {
		drawWrappedText(screen, a.launcherError, panelX+24, fieldY+64, panelW-48, 12, color.RGBA{R: 252, G: 165, B: 165, A: 255})
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
