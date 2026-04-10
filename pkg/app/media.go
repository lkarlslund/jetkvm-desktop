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
	"github.com/sqweek/dialog"

	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
	"github.com/lkarlslund/jetkvm-desktop/pkg/ui"
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
	a.mediaPanel = rect{}
	a.mediaButtons = a.mediaButtons[:0]
	ctx := a.newUIContext(screen, func(btn chromeButton) {
		a.mediaButtons = append(a.mediaButtons, btn)
	})
	ui.Inset{
		Insets: ui.UniformInsets(28),
		Child: ui.Align{
			Horizontal: ui.AlignCenter,
			Vertical:   ui.AlignCenter,
			Child: ui.Constrained{
				MaxW: 820,
				MaxH: 560,
				Child: mediaOverlayElement{
					app:  a,
					snap: snap,
				},
			},
		},
	}.Draw(ctx, ui.Rect{W: float64(bounds.Dx()), H: float64(bounds.Dy())})
}

type mediaOverlayElement struct {
	app  *App
	snap session.Snapshot
}

func (mediaOverlayElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e mediaOverlayElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	e.app.mediaPanel = rect{x: bounds.X, y: bounds.Y, w: bounds.W, h: bounds.H}
	ctx.FillRect(ui.Rect{W: float64(ctx.Screen.Bounds().Dx()), H: float64(ctx.Screen.Bounds().Dy())}, ctx.Theme.Backdrop)
	ui.Panel{
		Fill:   ctx.Theme.ModalFill,
		Stroke: ctx.Theme.ModalStroke,
		Insets: ui.UniformInsets(18),
		Child: ui.Column{
			Children: []ui.Child{
				ui.Fixed(mediaHeaderElement{app: e.app}),
				ui.Fixed(ui.Spacer{H: 18}),
				ui.Fixed(ui.Panel{
					Fill:   ctx.Theme.SectionFill,
					Stroke: ctx.Theme.SectionStroke,
					Insets: ui.UniformInsets(16),
					Child:  mediaStateElement{app: e.app},
				}),
				ui.Fixed(ui.Spacer{H: 18}),
				ui.Fixed(mediaTabsElement{app: e.app}),
				ui.Fixed(ui.Spacer{H: 14}),
				ui.Flex(ui.Panel{
					Fill:   ctx.Theme.PanelFill,
					Stroke: ctx.Theme.PanelStroke,
					Insets: ui.UniformInsets(18),
					Child:  e.app.mediaBodyElement(e.snap),
				}, 1),
			},
		},
	}.Draw(ctx, bounds)
}

func (a *App) mediaBodyElement(snap session.Snapshot) ui.Element {
	switch a.mediaView {
	case mediaViewURL:
		return mediaURLBodyElement{app: a}
	case mediaViewStorage:
		return mediaStorageBodyElement{app: a}
	case mediaViewUpload:
		return mediaUploadBodyElement{app: a}
	default:
		return mediaHomeBodyElement{app: a, snap: snap}
	}
}

type mediaHeaderElement struct {
	app *App
}

func (mediaHeaderElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: 52})
}

func (h mediaHeaderElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	title := ui.Column{
		Children: []ui.Child{
			ui.Fixed(ui.Label{Text: "Virtual Media", Size: 26, Color: ctx.Theme.Title}),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Paragraph{
				Text:  "Mount an image by URL, use JetKVM storage, or upload an ISO/IMG from this computer.",
				Size:  12,
				Color: ctx.Theme.Muted,
			}),
		},
	}
	rightChildren := []ui.Child{}
	if h.app.mediaLoading {
		rightChildren = append(rightChildren, ui.Fixed(ui.Label{Text: "Working…", Size: 12, Color: color.RGBA{R: 147, G: 197, B: 253, A: 255}}), ui.Fixed(ui.Spacer{H: 10}))
	}
	rightChildren = append(rightChildren, ui.Fixed(ui.Button{ID: "media_close", Label: "X", Enabled: !h.app.mediaUploading}))
	ui.Row{
		Children: []ui.Child{
			ui.Flex(title, 1),
			ui.Fixed(ui.Column{Children: rightChildren}),
		},
		Spacing: 12,
	}.Draw(ctx, bounds)
}

type mediaStateElement struct {
	app *App
}

func (mediaStateElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: 80})
}

func (e mediaStateElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	if e.app.mediaState == nil {
		ui.Column{
			Children: []ui.Child{
				ui.Fixed(ui.Label{Text: "Current mount", Size: 15, Color: ctx.Theme.Title}),
				ui.Fixed(ui.Spacer{H: 16}),
				ui.Fixed(ui.Label{Text: "Nothing mounted", Size: 18, Color: ctx.Theme.Title}),
				ui.Fixed(ui.Spacer{H: 12}),
				ui.Fixed(ui.Paragraph{
					Text:  "Choose a source below to expose media to the remote host.",
					Size:  12,
					Color: ctx.Theme.Muted,
				}),
			},
		}.Draw(ctx, bounds)
		return
	}
	label := e.app.mediaState.Filename
	if label == "" {
		label = e.app.mediaState.URL
	}
	ui.Column{
		Children: []ui.Child{
			ui.Fixed(ui.Label{Text: "Current mount", Size: 15, Color: ctx.Theme.Title}),
			ui.Fixed(ui.Spacer{H: 16}),
			ui.Fixed(ui.Row{
				Children: []ui.Child{
					ui.Fixed(ui.Column{
						Children: []ui.Child{
							ui.Fixed(ui.KeyValue{Label: "Source", Value: string(e.app.mediaState.Source), LabelWidth: 74}),
							ui.Fixed(ui.Spacer{H: 10}),
							ui.Fixed(ui.KeyValue{Label: "Mode", Value: string(e.app.mediaState.Mode), LabelWidth: 74}),
						},
					}),
					ui.Fixed(ui.Spacer{W: 24}),
					ui.Flex(ui.Column{
						Children: []ui.Child{
							ui.Fixed(ui.Paragraph{Text: fallbackLabel(label, "Mounted media"), Size: 12, Color: ctx.Theme.Body}),
							ui.Fixed(ui.Spacer{H: 10}),
							ui.Fixed(ui.Label{Text: humanBytes(e.app.mediaState.Size), Size: 12, Color: ctx.Theme.Muted}),
						},
					}, 1),
					ui.Fixed(ui.Button{
						ID:      "media_unmount",
						Label:   "Unmount",
						Enabled: !e.app.mediaLoading && !e.app.mediaUploading,
					}),
				},
				Spacing: 0,
			}),
		},
	}.Draw(ctx, bounds)
}

type mediaTabsElement struct {
	app *App
}

func (mediaTabsElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: 30})
}

func (e mediaTabsElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Row{
		Children: []ui.Child{
			ui.Fixed(ui.Button{ID: "media_view_home", Label: "Overview", Enabled: !e.app.mediaUploading, Active: e.app.mediaView == mediaViewHome}),
			ui.Fixed(ui.Button{ID: "media_view_url", Label: "URL", Enabled: !e.app.mediaUploading, Active: e.app.mediaView == mediaViewURL}),
			ui.Fixed(ui.Button{ID: "media_view_storage", Label: "Storage", Enabled: !e.app.mediaUploading, Active: e.app.mediaView == mediaViewStorage}),
			ui.Fixed(ui.Button{ID: "media_view_upload", Label: "Upload", Enabled: !e.app.mediaUploading, Active: e.app.mediaView == mediaViewUpload}),
		},
		Spacing: 10,
	}.Draw(ctx, bounds)
}

type mediaHomeBodyElement struct {
	app  *App
	snap session.Snapshot
}

func (e mediaHomeBodyElement) Measure(ctx *ui.Context, constraints ui.Constraints) ui.Size {
	return mediaBodyColumn(e.content()).Measure(ctx, constraints)
}

func (e mediaHomeBodyElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	mediaBodyColumn(e.content()).Draw(ctx, bounds)
}

func (e mediaHomeBodyElement) content() []ui.Child {
	return []ui.Child{
		ui.Fixed(ui.Label{Text: "Choose a source", Size: 18, Color: ui.DefaultTheme().Title}),
		ui.Fixed(ui.Spacer{H: 12}),
		ui.Fixed(ui.Paragraph{
			Text:  "Use URL mounting for public ISOs, JetKVM storage for already-uploaded images, or Upload to send a local file to the device.",
			Size:  12,
			Color: ui.DefaultTheme().Muted,
		}),
		ui.Fixed(ui.Spacer{H: 16}),
		ui.Fixed(ui.KeyValue{Label: "Device", Value: fallbackLabel(e.snap.Hostname, e.snap.DeviceID, "Unknown"), LabelWidth: 72}),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.KeyValue{Label: "Storage Used", Value: humanBytes(e.app.mediaSpace.BytesUsed), LabelWidth: 96}),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.KeyValue{Label: "Storage Free", Value: humanBytes(e.app.mediaSpace.BytesFree), LabelWidth: 96}),
		ui.Flex(ui.Spacer{}, 1),
		ui.Fixed(ui.Label{Text: "Tips", Size: 16, Color: ui.DefaultTheme().Title}),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(ui.Paragraph{
			Text:  "ISO files normally want CDROM mode. IMG files usually want Disk mode. Only one piece of virtual media can be mounted at a time.",
			Size:  12,
			Color: ui.DefaultTheme().Muted,
		}),
	}
}

type mediaURLBodyElement struct {
	app *App
}

func (e mediaURLBodyElement) Measure(ctx *ui.Context, constraints ui.Constraints) ui.Size {
	return mediaBodyColumn(e.content()).Measure(ctx, constraints)
}

func (e mediaURLBodyElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	mediaBodyColumn(e.content()).Draw(ctx, bounds)
}

func (e mediaURLBodyElement) content() []ui.Child {
	children := []ui.Child{
		ui.Fixed(ui.Label{Text: "Mount from URL", Size: 18, Color: ui.DefaultTheme().Title}),
		ui.Fixed(ui.Spacer{H: 12}),
		ui.Fixed(ui.Paragraph{
			Text:  "Paste a direct ISO or IMG URL. The file stays remote; JetKVM streams it on demand.",
			Size:  12,
			Color: ui.DefaultTheme().Muted,
		}),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(ui.TextField{
			ID:          "media_focus_url",
			Value:       e.app.mediaURL,
			Placeholder: "https://example.com/image.iso",
			Focused:     e.app.mediaURLFocused,
			Enabled:     true,
		}),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(ui.Label{Text: "USB Mode", Size: 13, Color: ui.DefaultTheme().Muted}),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(mediaModeButtons{app: e.app, disabled: false}),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(ui.Button{ID: "media_mount_url", Label: "Mount URL", Enabled: e.app.canMountMediaURL()}),
	}
	if strings.TrimSpace(e.app.mediaURL) != "" && !isValidMediaURL(e.app.mediaURL) {
		children = append(children,
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(ui.Label{Text: "Enter a valid absolute URL.", Size: 12, Color: ui.DefaultTheme().Error}),
		)
	}
	children = append(children, ui.Flex(ui.Spacer{}, 1))
	if e.app.mediaError != "" {
		children = append(children, ui.Fixed(ui.Paragraph{Text: e.app.mediaError, Size: 12, Color: ui.DefaultTheme().Error}))
	}
	return children
}

type mediaStorageBodyElement struct {
	app *App
}

func (e mediaStorageBodyElement) Measure(ctx *ui.Context, constraints ui.Constraints) ui.Size {
	return mediaBodyColumn(e.content()).Measure(ctx, constraints)
}

func (e mediaStorageBodyElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	mediaBodyColumn(e.content()).Draw(ctx, bounds)
}

func (e mediaStorageBodyElement) content() []ui.Child {
	children := []ui.Child{
		ui.Fixed(ui.Label{Text: "JetKVM Storage", Size: 18, Color: ui.DefaultTheme().Title}),
		ui.Fixed(ui.Spacer{H: 12}),
		ui.Fixed(ui.Paragraph{
			Text:  "Select an image already stored on the device. Incomplete uploads are shown but cannot be mounted.",
			Size:  12,
			Color: ui.DefaultTheme().Muted,
		}),
		ui.Fixed(ui.Spacer{H: 16}),
	}
	if len(e.app.mediaFiles) == 0 {
		children = append(children, ui.Fixed(ui.Label{Text: "No stored images yet.", Size: 14, Color: ui.DefaultTheme().Muted}))
	} else {
		for i, file := range e.app.mediaFiles {
			if i >= 7 {
				break
			}
			if i > 0 {
				children = append(children, ui.Fixed(ui.Spacer{H: 8}))
			}
			children = append(children, ui.Fixed(mediaFileRowElement{
				app:  e.app,
				file: file,
			}))
		}
	}
	children = append(children,
		ui.Flex(ui.Spacer{}, 1),
		ui.Fixed(ui.Label{Text: "USB Mode", Size: 13, Color: ui.DefaultTheme().Muted}),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(ui.Row{
			Children: []ui.Child{
				ui.Fixed(mediaModeButtons{app: e.app, disabled: false}),
				ui.Flex(ui.Spacer{}, 1),
				ui.Fixed(ui.Button{ID: "media_mount_storage", Label: "Mount Selected", Enabled: e.app.canMountSelectedStorageFile()}),
			},
			Spacing: 10,
		}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(ui.Label{
			Text:  fmt.Sprintf("Used %s  Free %s", humanBytes(e.app.mediaSpace.BytesUsed), humanBytes(e.app.mediaSpace.BytesFree)),
			Size:  12,
			Color: ui.DefaultTheme().Muted,
		}),
	)
	if e.app.mediaError != "" {
		children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(ui.Paragraph{Text: e.app.mediaError, Size: 12, Color: ui.DefaultTheme().Error}))
	}
	return children
}

type mediaUploadBodyElement struct {
	app *App
}

func (e mediaUploadBodyElement) Measure(ctx *ui.Context, constraints ui.Constraints) ui.Size {
	return mediaBodyColumn(e.content()).Measure(ctx, constraints)
}

func (e mediaUploadBodyElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	mediaBodyColumn(e.content()).Draw(ctx, bounds)
}

func (e mediaUploadBodyElement) content() []ui.Child {
	children := []ui.Child{
		ui.Fixed(ui.Label{Text: "Upload Image", Size: 18, Color: ui.DefaultTheme().Title}),
		ui.Fixed(ui.Spacer{H: 12}),
		ui.Fixed(ui.Paragraph{
			Text:  "Pick a local ISO or IMG file and upload it into JetKVM storage. The same device storage browser can mount it afterward.",
			Size:  12,
			Color: ui.DefaultTheme().Muted,
		}),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(ui.Row{
			Children: []ui.Child{
				ui.Flex(ui.TextField{
					ID:          "media_focus_upload",
					Value:       e.app.mediaUploadPath,
					Placeholder: "/path/to/image.iso",
					Focused:     e.app.mediaUploadFocused,
					Enabled:     !e.app.mediaUploading,
				}, 1),
				ui.Fixed(ui.Button{ID: "media_browse_upload", Label: "Browse", Enabled: !e.app.mediaUploading}),
			},
			Spacing: 10,
		}),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(ui.Label{Text: "Mount Mode", Size: 13, Color: ui.DefaultTheme().Muted}),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(mediaModeButtons{app: e.app, disabled: e.app.mediaUploading}),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(ui.Button{
			ID:      "media_start_upload",
			Label:   "Start Upload",
			Enabled: !e.app.mediaUploading && strings.TrimSpace(e.app.mediaUploadPath) != "",
		}),
		ui.Flex(ui.Spacer{}, 1),
	}
	if e.app.mediaUploadTotal > 0 {
		children = append(children,
			ui.Fixed(ui.ProgressBar{Progress: e.app.mediaUploadProgress}),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(mediaUploadMetaElement(e)),
			ui.Fixed(ui.Spacer{H: 12}),
		)
	}
	if strings.TrimSpace(e.app.mediaUploadPath) != "" {
		children = append(children, ui.Fixed(ui.Label{
			Text:  trimTextToWidth("Selected: "+filepath.Base(e.app.mediaUploadPath), 640, 12),
			Size:  12,
			Color: ui.DefaultTheme().Muted,
		}))
	}
	if e.app.mediaError != "" {
		children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(ui.Paragraph{Text: e.app.mediaError, Size: 12, Color: ui.DefaultTheme().Error}))
	}
	return children
}

func mediaBodyColumn(children []ui.Child) ui.Element {
	return ui.Column{Children: children, Spacing: 0}
}

type mediaModeButtons struct {
	app      *App
	disabled bool
}

func (mediaModeButtons) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: 30})
}

func (e mediaModeButtons) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Row{
		Children: []ui.Child{
			ui.Fixed(ui.Button{ID: "media_mode_cdrom", Label: "CD/DVD", Enabled: !e.disabled, Active: e.app.mediaMode == virtualmedia.ModeCDROM}),
			ui.Fixed(ui.Button{ID: "media_mode_disk", Label: "Disk", Enabled: !e.disabled, Active: e.app.mediaMode == virtualmedia.ModeDisk}),
		},
		Spacing: 12,
	}.Draw(ctx, bounds)
}

type mediaFileRowElement struct {
	app  *App
	file mediaFileRow
}

func (mediaFileRowElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: 38})
}

func (e mediaFileRowElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	fill := color.RGBA{R: 18, G: 26, B: 38, A: 255}
	if e.app.mediaSelectedFile == e.file.Filename {
		fill = color.RGBA{R: 32, G: 74, B: 122, A: 255}
	}
	ui.Panel{
		Fill:   fill,
		Stroke: ctx.Theme.PanelStroke,
		Insets: ui.SymmetricInsets(12, 10),
		Child: ui.Row{
			Children: []ui.Child{
				ui.Flex(ui.Label{Text: e.file.Filename, Size: 13, Color: ctx.Theme.Body}, 1),
				ui.Fixed(ui.Label{Text: humanBytes(e.file.Size), Size: 12, Color: ctx.Theme.Muted}),
				ui.Fixed(ui.Spacer{W: 12}),
				ui.Fixed(ui.Button{
					ID:      "media_delete:" + e.file.Filename,
					Label:   "Delete",
					Enabled: !e.app.mediaUploading && !e.app.mediaLoading,
				}),
			},
			Spacing: 0,
		},
	}.Draw(ctx, bounds)
	ctx.AddHit("media_select:"+e.file.Filename, ui.Rect{X: bounds.X, Y: bounds.Y, W: bounds.W - 92, H: bounds.H}, true)
}

type mediaUploadMetaElement struct {
	app *App
}

func (mediaUploadMetaElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: 14})
}

func (e mediaUploadMetaElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	left := fmt.Sprintf("%s / %s", humanBytes(e.app.mediaUploadSent), humanBytes(e.app.mediaUploadTotal))
	children := []ui.Child{
		ui.Fixed(ui.Label{Text: left, Size: 12, Color: ctx.Theme.Body}),
	}
	if e.app.mediaUploadSpeed > 0 {
		speedLabel := fmt.Sprintf("%s/s", humanBytes(int64(e.app.mediaUploadSpeed)))
		if etaLabel := mediaUploadETA(e.app.mediaUploadSent, e.app.mediaUploadTotal, e.app.mediaUploadSpeed); etaLabel != "" {
			speedLabel += "  ETA " + etaLabel
		}
		children = append(children, ui.Flex(ui.Spacer{}, 1), ui.Fixed(ui.Label{Text: speedLabel, Size: 12, Color: ctx.Theme.Muted}))
	}
	ui.Row{Children: children, Spacing: 12}.Draw(ctx, bounds)
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

func trimTextToWidth(value string, width, size float64) string {
	if value == "" || ui.TextFits(value, width, size) {
		return value
	}
	runes := []rune(value)
	if len(runes) <= 3 {
		return value
	}
	left := len(runes) / 2
	right := left
	best := "..."
	for left > 0 && right < len(runes) {
		candidate := string(runes[:left]) + "..." + string(runes[right:])
		if ui.TextFits(candidate, width, size) {
			best = candidate
			left--
			right++
			continue
		}
		break
	}
	return best
}

func mediaUploadETA(sent, total int64, bytesPerS float64) string {
	if total <= sent || bytesPerS <= 0 {
		return ""
	}
	remainingSeconds := int64(float64(total-sent) / bytesPerS)
	switch {
	case remainingSeconds < 60:
		return fmt.Sprintf("%ds", remainingSeconds)
	case remainingSeconds < 3600:
		return fmt.Sprintf("%dm %02ds", remainingSeconds/60, remainingSeconds%60)
	default:
		return fmt.Sprintf("%dh %02dm", remainingSeconds/3600, (remainingSeconds%3600)/60)
	}
}
