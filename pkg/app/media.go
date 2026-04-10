package app

import (
	"context"
	"fmt"
	"image/color"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/sqweek/dialog"

	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
	"github.com/lkarlslund/jetkvm-desktop/pkg/virtualmedia"
)

func (a *App) openMediaOverlay() {
	if a.ctrl == nil || a.ctrl.Snapshot().Phase != session.PhaseConnected {
		return
	}
	a.mediaOpen = true
	a.mediaView = mediaViewHome
	a.pasteOpen = false
	a.settingsOpen = false
	a.mediaError = ""
	a.mediaURLFocused = false
	a.mediaUploadFocused = false
	a.applyCursorMode()
	a.refreshMediaData()
}

func (a *App) refreshMediaData() {
	if a.ctrl == nil || a.mediaUploading {
		return
	}
	a.mediaLoading = true
	a.runAsync(func() {
		err := a.reloadMediaData()
		a.mu.Lock()
		a.mediaLoading = false
		if err != nil {
			a.mediaError = err.Error()
		}
		a.mu.Unlock()
	})
}

func (a *App) reloadMediaData() error {
	if a.ctrl == nil {
		return fmt.Errorf("client not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	state, err := a.ctrl.GetVirtualMediaState(ctx)
	if err != nil {
		return err
	}
	files, err := a.ctrl.ListStorageFiles(ctx)
	if err != nil {
		return err
	}
	space, err := a.ctrl.GetStorageSpace(ctx)
	if err != nil {
		return err
	}

	fileRows := make([]mediaFileRow, 0, len(files))
	for _, file := range files {
		fileRows = append(fileRows, mediaFileRow{
			Filename:  file.Filename,
			Size:      file.Size,
			CreatedAt: file.CreatedAt,
		})
	}

	a.mu.Lock()
	a.mediaState = state
	a.mediaFiles = fileRows
	a.mediaSpace = mediaSpaceSnapshot{BytesUsed: space.BytesUsed, BytesFree: space.BytesFree}
	a.mediaStorageLoaded = true
	if a.mediaSelectedFile != "" && !a.mediaFileExistsLocked(a.mediaSelectedFile) {
		a.mediaSelectedFile = ""
	}
	if state != nil {
		a.mediaMode = state.Mode
	}
	a.mu.Unlock()
	return nil
}

func (a *App) mediaFileExistsLocked(name string) bool {
	for _, file := range a.mediaFiles {
		if file.Filename == name {
			return true
		}
	}
	return false
}

func (a *App) syncMediaInput() {
	if !a.mediaOpen || a.mediaUploading {
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		switch {
		case a.mediaURLFocused:
			runes := []rune(a.mediaURL)
			if len(runes) > 0 {
				a.mediaURL = string(runes[:len(runes)-1])
			}
		case a.mediaUploadFocused:
			runes := []rune(a.mediaUploadPath)
			if len(runes) > 0 {
				a.mediaUploadPath = string(runes[:len(runes)-1])
			}
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		switch {
		case a.mediaView == mediaViewURL && a.mediaURLFocused:
			a.invokeAction("media_mount_url")
		case a.mediaView == mediaViewUpload && a.mediaUploadFocused:
			a.invokeAction("media_start_upload")
		}
		return
	}
	for _, r := range ebiten.AppendInputChars(nil) {
		if r < 32 || r == 127 {
			continue
		}
		switch {
		case a.mediaURLFocused:
			a.mediaURL += string(r)
		case a.mediaUploadFocused:
			a.mediaUploadPath += string(r)
		}
	}
}

func (a *App) invokeMediaAction(id string) bool {
	switch {
	case id == "media_view_home":
		if !a.mediaUploading {
			a.mediaView = mediaViewHome
			a.mediaError = ""
		}
		return true
	case id == "media_view_url":
		if !a.mediaUploading {
			a.mediaView = mediaViewURL
			a.mediaError = ""
			a.mediaURLFocused = true
			a.mediaUploadFocused = false
		}
		return true
	case id == "media_view_storage":
		if !a.mediaUploading {
			a.mediaView = mediaViewStorage
			a.mediaError = ""
			a.mediaURLFocused = false
			a.mediaUploadFocused = false
			a.refreshMediaData()
		}
		return true
	case id == "media_view_upload":
		if !a.mediaUploading {
			a.mediaView = mediaViewUpload
			a.mediaError = ""
			a.mediaURLFocused = false
			a.mediaUploadFocused = true
			a.refreshMediaData()
		}
		return true
	case id == "media_mode_cdrom":
		a.mediaMode = virtualmedia.ModeCDROM
		return true
	case id == "media_mode_disk":
		a.mediaMode = virtualmedia.ModeDisk
		return true
	case id == "media_focus_url":
		a.mediaURLFocused = true
		a.mediaUploadFocused = false
		return true
	case id == "media_focus_upload":
		a.mediaUploadFocused = true
		a.mediaURLFocused = false
		return true
	case id == "media_unmount":
		if a.mediaUploading || a.ctrl == nil || a.mediaState == nil {
			return true
		}
		a.mediaLoading = true
		a.mediaError = ""
		a.runAsync(func() {
			err := a.ctrl.UnmountMedia()
			if err == nil {
				err = a.reloadMediaData()
			}
			a.mu.Lock()
			a.mediaLoading = false
			if err != nil {
				a.mediaError = err.Error()
			}
			a.mu.Unlock()
		})
		return true
	case id == "media_mount_url":
		if a.mediaUploading || a.ctrl == nil || !a.canMountMediaURL() {
			return true
		}
		targetURL := strings.TrimSpace(a.mediaURL)
		mode := a.mediaMode
		a.mediaLoading = true
		a.mediaError = ""
		a.runAsync(func() {
			err := a.ctrl.MountMediaURL(targetURL, mode)
			if err == nil {
				err = a.reloadMediaData()
			}
			a.mu.Lock()
			a.mediaLoading = false
			if err != nil {
				a.mediaError = err.Error()
			} else {
				a.mediaView = mediaViewHome
			}
			a.mu.Unlock()
		})
		return true
	case id == "media_mount_storage":
		if a.mediaUploading || a.ctrl == nil || !a.canMountSelectedStorageFile() {
			return true
		}
		filename := a.mediaSelectedFile
		mode := a.mediaMode
		a.mediaLoading = true
		a.mediaError = ""
		a.runAsync(func() {
			err := a.ctrl.MountStorageFile(filename, mode)
			if err == nil {
				err = a.reloadMediaData()
			}
			a.mu.Lock()
			a.mediaLoading = false
			if err != nil {
				a.mediaError = err.Error()
			} else {
				a.mediaView = mediaViewHome
			}
			a.mu.Unlock()
		})
		return true
	case id == "media_delete_selected":
		if a.mediaUploading || a.ctrl == nil || strings.TrimSpace(a.mediaSelectedFile) == "" {
			return true
		}
		filename := a.mediaSelectedFile
		a.mediaLoading = true
		a.mediaError = ""
		a.runAsync(func() {
			err := a.ctrl.DeleteStorageFile(filename)
			if err == nil {
				err = a.reloadMediaData()
			}
			a.mu.Lock()
			a.mediaLoading = false
			if err != nil {
				a.mediaError = err.Error()
			}
			a.mu.Unlock()
		})
		return true
	case id == "media_browse_upload":
		if !a.mediaUploading {
			a.pickUploadFile()
		}
		return true
	case id == "media_start_upload":
		if a.mediaUploading || a.ctrl == nil || strings.TrimSpace(a.mediaUploadPath) == "" {
			return true
		}
		path := a.mediaUploadPath
		a.mediaUploading = true
		a.mediaError = ""
		a.mediaUploadProgress = 0
		a.mediaUploadSent = 0
		a.mediaUploadTotal = 0
		a.mediaUploadSpeed = 0
		a.runAsync(func() {
			err := a.ctrl.UploadStorageFile(path, func(progress virtualmedia.UploadProgress) {
				a.mu.Lock()
				a.mediaUploadSent = progress.Sent
				a.mediaUploadTotal = progress.Total
				if progress.Total > 0 {
					a.mediaUploadProgress = float64(progress.Sent) / float64(progress.Total)
				}
				a.mediaUploadSpeed = progress.BytesPerS
				a.mu.Unlock()
			})
			if err == nil {
				err = a.reloadMediaData()
			}
			a.mu.Lock()
			a.mediaUploading = false
			if err != nil {
				a.mediaError = err.Error()
			} else {
				a.mediaView = mediaViewStorage
			}
			a.mu.Unlock()
		})
		return true
	case strings.HasPrefix(id, "media_select:"):
		a.mediaSelectedFile = strings.TrimPrefix(id, "media_select:")
		if strings.HasSuffix(strings.ToLower(a.mediaSelectedFile), ".img") {
			a.mediaMode = virtualmedia.ModeDisk
		} else {
			a.mediaMode = virtualmedia.ModeCDROM
		}
		return true
	case strings.HasPrefix(id, "media_delete:"):
		if a.mediaUploading || a.ctrl == nil {
			return true
		}
		a.mediaSelectedFile = strings.TrimPrefix(id, "media_delete:")
		a.invokeMediaAction("media_delete_selected")
		return true
	}
	return false
}

func (a *App) pickUploadFile() {
	path, err := dialog.File().
		Title("Choose disk image").
		Filter("Disk images", "iso", "img").
		Load()
	if err != nil {
		if err == dialog.ErrCancelled {
			return
		}
		a.mediaError = err.Error()
		return
	}
	a.mediaUploadPath = path
	a.mediaUploadFocused = true
	a.mediaURLFocused = false
	if strings.HasSuffix(strings.ToLower(path), ".img") {
		a.mediaMode = virtualmedia.ModeDisk
	} else {
		a.mediaMode = virtualmedia.ModeCDROM
	}
}

func (a *App) canMountMediaURL() bool {
	return a.mediaState == nil && isValidMediaURL(a.mediaURL) && !a.mediaLoading
}

func (a *App) canMountSelectedStorageFile() bool {
	return a.mediaState == nil &&
		a.mediaSelectedFile != "" &&
		!strings.HasSuffix(strings.ToLower(a.mediaSelectedFile), ".incomplete") &&
		!a.mediaLoading
}

func isValidMediaURL(raw string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

func (a *App) drawMediaOverlay(screen *ebiten.Image, snap session.Snapshot) {
	if !a.mediaOpen {
		a.mediaButtons = nil
		a.mediaPanel = rect{}
		return
	}
	bounds := screen.Bounds()
	panelW := min(820, float64(bounds.Dx()-56))
	panelH := min(560, float64(bounds.Dy()-56))
	panelX := float64(bounds.Dx())/2 - panelW/2
	panelY := float64(bounds.Dy())/2 - panelH/2
	a.mediaPanel = rect{x: panelX, y: panelY, w: panelW, h: panelH}
	a.mediaButtons = a.mediaButtons[:0]

	vector.FillRect(screen, 0, 0, float32(bounds.Dx()), float32(bounds.Dy()), color.RGBA{A: 160}, false)
	vector.FillRect(screen, float32(panelX), float32(panelY), float32(panelW), float32(panelH), color.RGBA{R: 10, G: 16, B: 24, A: 244}, false)
	vector.StrokeRect(screen, float32(panelX), float32(panelY), float32(panelW), float32(panelH), 1, color.RGBA{R: 110, G: 130, B: 152, A: 110}, false)

	drawText(screen, "Virtual Media", panelX+18, panelY+18, 26, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawWrappedText(screen, "Mount an image by URL, use JetKVM storage, or upload an ISO/IMG from this computer.", panelX+18, panelY+52, panelW-72, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	a.drawMediaButton(screen, chromeButton{
		id:      "media_close",
		label:   "X",
		enabled: !a.mediaUploading,
		rect:    rect{x: panelX + panelW - 38, y: panelY + 12, w: 24, h: 24},
	}, false)

	stateY := panelY + 88
	stateH := 112.0
	vector.FillRect(screen, float32(panelX+18), float32(stateY), float32(panelW-36), float32(stateH), color.RGBA{R: 18, G: 26, B: 38, A: 236}, false)
	vector.StrokeRect(screen, float32(panelX+18), float32(stateY), float32(panelW-36), float32(stateH), 1, color.RGBA{R: 94, G: 115, B: 136, A: 110}, false)
	drawText(screen, "Current mount", panelX+34, stateY+16, 15, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	if a.mediaState == nil {
		drawText(screen, "Nothing mounted", panelX+34, stateY+48, 18, color.RGBA{R: 236, G: 241, B: 245, A: 255})
		drawWrappedText(screen, "Choose a source below to expose media to the remote host.", panelX+34, stateY+76, panelW-220, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	} else {
		source := a.mediaState.Source
		label := a.mediaState.Filename
		if label == "" {
			label = a.mediaState.URL
		}
		drawSettingsKeyValue(screen, "Source", string(source), panelX+34, stateY+44, 74)
		drawSettingsKeyValue(screen, "Mode", string(a.mediaState.Mode), panelX+34, stateY+70, 74)
		drawWrappedText(screen, fallbackLabel(label, "Mounted media"), panelX+200, stateY+44, panelW-330, 12, color.RGBA{R: 236, G: 241, B: 245, A: 255})
		drawText(screen, humanBytes(a.mediaState.Size), panelX+200, stateY+78, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
		a.drawMediaButton(screen, chromeButton{
			id:      "media_unmount",
			label:   "Unmount",
			enabled: !a.mediaLoading && !a.mediaUploading,
			rect:    rect{x: panelX + panelW - 132, y: stateY + 38, w: 92, h: 34},
		}, false)
	}

	tabY := stateY + stateH + 18
	tabX := panelX + 18
	tabDefs := []struct {
		id, label string
		active    bool
		w         float64
	}{
		{id: "media_view_home", label: "Overview", active: a.mediaView == mediaViewHome, w: 96},
		{id: "media_view_url", label: "URL", active: a.mediaView == mediaViewURL, w: 76},
		{id: "media_view_storage", label: "Storage", active: a.mediaView == mediaViewStorage, w: 92},
		{id: "media_view_upload", label: "Upload", active: a.mediaView == mediaViewUpload, w: 92},
	}
	for _, tab := range tabDefs {
		a.drawMediaButton(screen, chromeButton{
			id:      tab.id,
			label:   tab.label,
			enabled: !a.mediaUploading,
			active:  tab.active,
			rect:    rect{x: tabX, y: tabY, w: tab.w, h: 30},
		}, tab.active)
		tabX += tab.w + 10
	}

	bodyX := panelX + 18
	bodyY := tabY + 44
	bodyW := panelW - 36
	bodyH := panelH - (bodyY - panelY) - 18
	vector.FillRect(screen, float32(bodyX), float32(bodyY), float32(bodyW), float32(bodyH), color.RGBA{R: 14, G: 22, B: 32, A: 234}, false)
	vector.StrokeRect(screen, float32(bodyX), float32(bodyY), float32(bodyW), float32(bodyH), 1, color.RGBA{R: 84, G: 104, B: 122, A: 110}, false)

	switch a.mediaView {
	case mediaViewURL:
		a.drawMediaURLView(screen, bodyX, bodyY, bodyW)
	case mediaViewStorage:
		a.drawMediaStorageView(screen, bodyX, bodyY, bodyW, bodyH)
	case mediaViewUpload:
		a.drawMediaUploadView(screen, bodyX, bodyY, bodyW)
	default:
		a.drawMediaHomeView(screen, bodyX, bodyY, bodyW, snap)
	}

	if a.mediaLoading {
		drawText(screen, "Working…", panelX+panelW-106, panelY+56, 12, color.RGBA{R: 147, G: 197, B: 253, A: 255})
	}
	if a.mediaError != "" {
		drawWrappedText(screen, a.mediaError, bodyX+18, bodyY+bodyH-40, bodyW-36, 12, color.RGBA{R: 252, G: 165, B: 165, A: 255})
	}
}

func (a *App) drawMediaHomeView(screen *ebiten.Image, x, y, w float64, snap session.Snapshot) {
	drawText(screen, "Choose a source", x+18, y+18, 18, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawWrappedText(screen, "Use URL mounting for public ISOs, JetKVM storage for already-uploaded images, or Upload to send a local file to the device.", x+18, y+46, w-36, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawSettingsKeyValue(screen, "Device", fallbackLabel(snap.Hostname, snap.DeviceID, "Unknown"), x+18, y+96, 72)
	drawSettingsKeyValue(screen, "Storage Used", humanBytes(a.mediaSpace.BytesUsed), x+18, y+122, 96)
	drawSettingsKeyValue(screen, "Storage Free", humanBytes(a.mediaSpace.BytesFree), x+18, y+148, 96)
	drawText(screen, "Tips", x+18, y+196, 16, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawWrappedText(screen, "ISO files normally want CDROM mode. IMG files usually want Disk mode. Only one piece of virtual media can be mounted at a time.", x+18, y+222, w-36, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
}

func (a *App) drawMediaURLView(screen *ebiten.Image, x, y, w float64) {
	drawText(screen, "Mount from URL", x+18, y+18, 18, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawWrappedText(screen, "Paste a direct ISO or IMG URL. The file stays remote; JetKVM streams it on demand.", x+18, y+46, w-36, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	a.drawMediaInput(screen, "media_focus_url", x+18, y+92, w-36, 38, a.mediaURL, "https://example.com/image.iso", a.mediaURLFocused)
	drawSettingsSectionLabel(screen, "USB Mode", x+18, y+152)
	a.drawMediaButton(screen, chromeButton{id: "media_mode_cdrom", label: "CD/DVD", enabled: true, active: a.mediaMode == virtualmedia.ModeCDROM, rect: rect{x: x + 108, y: y + 140, w: 90, h: 30}}, a.mediaMode == virtualmedia.ModeCDROM)
	a.drawMediaButton(screen, chromeButton{id: "media_mode_disk", label: "Disk", enabled: true, active: a.mediaMode == virtualmedia.ModeDisk, rect: rect{x: x + 210, y: y + 140, w: 72, h: 30}}, a.mediaMode == virtualmedia.ModeDisk)
	a.drawMediaButton(screen, chromeButton{id: "media_mount_url", label: "Mount URL", enabled: a.canMountMediaURL(), rect: rect{x: x + 18, y: y + 192, w: 114, h: 34}}, false)
	if strings.TrimSpace(a.mediaURL) != "" && !isValidMediaURL(a.mediaURL) {
		drawText(screen, "Enter a valid absolute URL.", x+18, y+240, 12, color.RGBA{R: 252, G: 165, B: 165, A: 255})
	}
}

func (a *App) drawMediaStorageView(screen *ebiten.Image, x, y, w, h float64) {
	drawText(screen, "JetKVM Storage", x+18, y+18, 18, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawWrappedText(screen, "Select an image already stored on the device. Incomplete uploads are shown but cannot be mounted.", x+18, y+46, w-36, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	listY := y + 88
	rowY := listY
	maxRows := 7
	for i, file := range a.mediaFiles {
		if i >= maxRows {
			break
		}
		active := a.mediaSelectedFile == file.Filename
		rowRect := rect{x: x + 18, y: rowY, w: w - 36, h: 38}
		fill := color.RGBA{R: 18, G: 26, B: 38, A: 255}
		if active {
			fill = color.RGBA{R: 32, G: 74, B: 122, A: 255}
		}
		vector.FillRect(screen, float32(rowRect.x), float32(rowRect.y), float32(rowRect.w), float32(rowRect.h), fill, false)
		vector.StrokeRect(screen, float32(rowRect.x), float32(rowRect.y), float32(rowRect.w), float32(rowRect.h), 1, color.RGBA{R: 84, G: 104, B: 122, A: 110}, false)
		a.mediaButtons = append(a.mediaButtons, chromeButton{id: "media_select:" + file.Filename, enabled: true, rect: rect{x: rowRect.x, y: rowRect.y, w: rowRect.w - 92, h: rowRect.h}})
		drawText(screen, file.Filename, rowRect.x+12, rowRect.y+10, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
		drawText(screen, humanBytes(file.Size), rowRect.x+rowRect.w-200, rowRect.y+10, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
		a.drawMediaButton(screen, chromeButton{
			id:      "media_delete:" + file.Filename,
			label:   "Delete",
			enabled: !a.mediaUploading && !a.mediaLoading,
			rect:    rect{x: rowRect.x + rowRect.w - 80, y: rowRect.y + 4, w: 64, h: 28},
		}, false)
		rowY += 46
	}
	if len(a.mediaFiles) == 0 {
		drawText(screen, "No stored images yet.", x+18, listY+12, 14, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	}
	drawSettingsSectionLabel(screen, "USB Mode", x+18, y+h-106)
	a.drawMediaButton(screen, chromeButton{id: "media_mode_cdrom", label: "CD/DVD", enabled: true, active: a.mediaMode == virtualmedia.ModeCDROM, rect: rect{x: x + 108, y: y + h - 118, w: 90, h: 30}}, a.mediaMode == virtualmedia.ModeCDROM)
	a.drawMediaButton(screen, chromeButton{id: "media_mode_disk", label: "Disk", enabled: true, active: a.mediaMode == virtualmedia.ModeDisk, rect: rect{x: x + 210, y: y + h - 118, w: 72, h: 30}}, a.mediaMode == virtualmedia.ModeDisk)
	a.drawMediaButton(screen, chromeButton{id: "media_mount_storage", label: "Mount Selected", enabled: a.canMountSelectedStorageFile(), rect: rect{x: x + w - 162, y: y + h - 122, w: 128, h: 34}}, false)
	drawText(screen, fmt.Sprintf("Used %s  Free %s", humanBytes(a.mediaSpace.BytesUsed), humanBytes(a.mediaSpace.BytesFree)), x+18, y+h-62, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
}

func (a *App) drawMediaUploadView(screen *ebiten.Image, x, y, w float64) {
	drawText(screen, "Upload Image", x+18, y+18, 18, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawWrappedText(screen, "Pick a local ISO or IMG file and upload it into JetKVM storage. The same device storage browser can mount it afterward.", x+18, y+46, w-36, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	a.drawMediaInput(screen, "media_focus_upload", x+18, y+96, w-156, 38, a.mediaUploadPath, "/path/to/image.iso", a.mediaUploadFocused)
	a.drawMediaButton(screen, chromeButton{id: "media_browse_upload", label: "Browse", enabled: !a.mediaUploading, rect: rect{x: x + w - 126, y: y + 96, w: 108, h: 38}}, false)
	drawSettingsSectionLabel(screen, "Mount Mode", x+18, y+162)
	a.drawMediaButton(screen, chromeButton{id: "media_mode_cdrom", label: "CD/DVD", enabled: !a.mediaUploading, active: a.mediaMode == virtualmedia.ModeCDROM, rect: rect{x: x + 108, y: y + 150, w: 90, h: 30}}, a.mediaMode == virtualmedia.ModeCDROM)
	a.drawMediaButton(screen, chromeButton{id: "media_mode_disk", label: "Disk", enabled: !a.mediaUploading, active: a.mediaMode == virtualmedia.ModeDisk, rect: rect{x: x + 210, y: y + 150, w: 72, h: 30}}, a.mediaMode == virtualmedia.ModeDisk)
	a.drawMediaButton(screen, chromeButton{id: "media_start_upload", label: "Start Upload", enabled: !a.mediaUploading && strings.TrimSpace(a.mediaUploadPath) != "", rect: rect{x: x + 18, y: y + 206, w: 118, h: 34}}, false)
	if a.mediaUploadTotal > 0 {
		barY := y + 264.0
		vector.FillRect(screen, float32(x+18), float32(barY), float32(w-36), 18, color.RGBA{R: 22, G: 30, B: 44, A: 255}, false)
		vector.FillRect(screen, float32(x+18), float32(barY), float32((w-36)*a.mediaUploadProgress), 18, color.RGBA{R: 48, G: 123, B: 206, A: 255}, false)
		vector.StrokeRect(screen, float32(x+18), float32(barY), float32(w-36), 18, 1, color.RGBA{R: 84, G: 104, B: 122, A: 110}, false)
		drawText(screen, fmt.Sprintf("%s / %s", humanBytes(a.mediaUploadSent), humanBytes(a.mediaUploadTotal)), x+18, y+292, 12, color.RGBA{R: 236, G: 241, B: 245, A: 255})
		if a.mediaUploadSpeed > 0 {
			drawText(screen, fmt.Sprintf("%s/s", humanBytes(int64(a.mediaUploadSpeed))), x+200, y+292, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
		}
	}
	if strings.TrimSpace(a.mediaUploadPath) != "" {
		drawText(screen, "Selected: "+filepath.Base(a.mediaUploadPath), x+18, y+330, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	}
}

func (a *App) drawMediaInput(screen *ebiten.Image, id string, x, y, w, h float64, value, placeholder string, focused bool) {
	border := color.RGBA{R: 84, G: 104, B: 122, A: 120}
	if focused {
		border = color.RGBA{R: 96, G: 165, B: 250, A: 180}
	}
	vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), color.RGBA{R: 8, G: 12, B: 18, A: 255}, false)
	vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 1, border, false)
	a.mediaButtons = append(a.mediaButtons, chromeButton{id: id, enabled: true, rect: rect{x: x, y: y, w: w, h: h}})
	text := value
	textColor := color.RGBA{R: 236, G: 241, B: 245, A: 255}
	if strings.TrimSpace(text) == "" {
		text = placeholder
		textColor = color.RGBA{R: 106, G: 120, B: 138, A: 255}
	}
	if focused && strings.TrimSpace(value) != "" && time.Now().UnixNano()/500_000_000%2 == 0 {
		text += "|"
	}
	drawText(screen, text, x+12, y+12, 13, textColor)
}

func (a *App) drawMediaButton(screen *ebiten.Image, btn chromeButton, active bool) {
	fill := color.RGBA{R: 18, G: 26, B: 38, A: 255}
	stroke := color.RGBA{R: 84, G: 104, B: 122, A: 120}
	label := color.RGBA{R: 236, G: 241, B: 245, A: 255}
	if active {
		fill = color.RGBA{R: 34, G: 78, B: 130, A: 255}
		stroke = color.RGBA{R: 147, G: 197, B: 253, A: 180}
	}
	if !btn.enabled {
		fill = color.RGBA{R: 18, G: 24, B: 32, A: 200}
		label = color.RGBA{R: 110, G: 120, B: 132, A: 255}
	}
	vector.FillRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), fill, false)
	vector.StrokeRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), 1, stroke, false)
	a.mediaButtons = append(a.mediaButtons, btn)
	tw, th := measureText(btn.label, 13)
	drawText(screen, btn.label, btn.rect.x+(btn.rect.w-tw)/2, btn.rect.y+(btn.rect.h-th)/2, 13, label)
}

func humanBytes(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	v := float64(value)
	for _, unit := range units {
		v /= 1024
		if v < 1024 {
			if v >= 10 {
				return fmt.Sprintf("%.0f %s", v, unit)
			}
			return fmt.Sprintf("%.1f %s", v, unit)
		}
	}
	return fmt.Sprintf("%.1f PB", v/1024)
}
