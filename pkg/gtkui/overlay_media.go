package gtkui

import (
	"context"
	"fmt"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/virtualmedia"
)

// MediaOverlay handles virtual media mounting (URL / storage / upload).
type MediaOverlay struct {
	Box *gtk.Box

	app *Application

	// Current mount
	mountLabel   *gtk.Label
	unmountBtn   *gtk.Button

	// Tabs
	tabStack     *gtk.Stack
	tabBar       *gtk.StackSwitcher

	// URL tab
	urlEntry     *gtk.Entry
	urlModeCD    *gtk.ToggleButton
	urlModeDisk  *gtk.ToggleButton
	mountURLBtn  *gtk.Button

	// Storage tab
	storageList  *gtk.ListBox
	storageMountBtn *gtk.Button
	storageModeCD   *gtk.ToggleButton
	storageModeDisk *gtk.ToggleButton

	// Upload tab
	pathEntry     *gtk.Entry
	browseBtn     *gtk.Button
	uploadModeCD  *gtk.ToggleButton
	uploadModeDisk *gtk.ToggleButton
	uploadBtn     *gtk.Button
	progressBar   *gtk.ProgressBar
	progressLabel *gtk.Label

	selectedFile string
}

func NewMediaOverlay(app *Application) *MediaOverlay {
	m := &MediaOverlay{app: app}
	m.Box = gtk.NewBox(gtk.OrientationVertical, 8)
	m.Box.AddCSSClass("overlay-panel")
	m.Box.SetHAlign(gtk.AlignCenter)
	m.Box.SetVAlign(gtk.AlignCenter)

	// Header
	header := gtk.NewBox(gtk.OrientationHorizontal, 8)
	title := gtk.NewLabel("Virtual Media")
	title.AddCSSClass("title-3")
	title.SetHExpand(true)
	title.SetXAlign(0)

	closeBtn := gtk.NewButtonFromIconName("window-close-symbolic")
	closeBtn.AddCSSClass("flat")
	closeBtn.ConnectClicked(func() { app.closeOverlay() })
	header.Append(title)
	header.Append(closeBtn)
	m.Box.Append(header)

	// Current mount info
	m.mountLabel = gtk.NewLabel("Nothing mounted")
	m.mountLabel.SetXAlign(0)
	m.mountLabel.AddCSSClass("dim-label")
	m.Box.Append(m.mountLabel)

	m.unmountBtn = gtk.NewButtonWithLabel("Unmount")
	m.unmountBtn.AddCSSClass("destructive-action")
	m.unmountBtn.SetVisible(false)
	m.unmountBtn.ConnectClicked(func() {
		if app.ctrl != nil {
			_ = app.ctrl.UnmountMedia()
		}
	})
	m.Box.Append(m.unmountBtn)

	// Tabs
	m.tabStack = gtk.NewStack()

	m.buildOverviewTab()
	m.buildURLTab()
	m.buildStorageTab()
	m.buildUploadTab()

	m.tabBar = gtk.NewStackSwitcher()
	m.tabBar.SetStack(m.tabStack)
	m.Box.Append(m.tabBar)
	m.Box.Append(m.tabStack)

	return m
}

func (m *MediaOverlay) buildOverviewTab() {
	box := gtk.NewBox(gtk.OrientationVertical, 4)
	box.SetMarginTop(8)
	info := gtk.NewLabel("Mount ISO or IMG files as virtual USB CD/DVD or Disk devices.\n\nUse the URL tab to mount from a network URL.\nUse Storage to mount files already on the device.\nUse Upload to transfer a file from this machine.")
	info.SetWrap(true)
	info.SetXAlign(0)
	box.Append(info)
	m.tabStack.AddTitled(box, "overview", "Overview")
}

func (m *MediaOverlay) buildURLTab() {
	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(8)

	m.urlEntry = gtk.NewEntry()
	m.urlEntry.SetPlaceholderText("https://example.com/image.iso")
	box.Append(m.urlEntry)

	modeBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	modeLabel := gtk.NewLabel("USB Mode:")
	modeLabel.AddCSSClass("dim-label")
	m.urlModeCD = gtk.NewToggleButton()
	m.urlModeCD.SetLabel("CD/DVD")
	m.urlModeCD.SetActive(true)
	m.urlModeDisk = gtk.NewToggleButton()
	m.urlModeDisk.SetLabel("Disk")
	m.urlModeDisk.SetGroup(m.urlModeCD)
	modeBox.Append(modeLabel)
	modeBox.Append(m.urlModeCD)
	modeBox.Append(m.urlModeDisk)
	box.Append(modeBox)

	m.mountURLBtn = gtk.NewButtonWithLabel("Mount URL")
	m.mountURLBtn.AddCSSClass("suggested-action")
	m.mountURLBtn.ConnectClicked(func() { m.mountURL() })
	box.Append(m.mountURLBtn)

	m.tabStack.AddTitled(box, "url", "URL")
}

func (m *MediaOverlay) buildStorageTab() {
	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(8)

	m.storageList = gtk.NewListBox()
	m.storageList.SetSelectionMode(gtk.SelectionSingle)
	m.storageList.AddCSSClass("boxed-list")

	scroll := gtk.NewScrolledWindow()
	scroll.SetChild(m.storageList)
	scroll.SetMinContentHeight(150)
	box.Append(scroll)

	modeBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	m.storageModeCD = gtk.NewToggleButton()
	m.storageModeCD.SetLabel("CD/DVD")
	m.storageModeCD.SetActive(true)
	m.storageModeDisk = gtk.NewToggleButton()
	m.storageModeDisk.SetLabel("Disk")
	m.storageModeDisk.SetGroup(m.storageModeCD)
	modeBox.Append(m.storageModeCD)
	modeBox.Append(m.storageModeDisk)

	m.storageMountBtn = gtk.NewButtonWithLabel("Mount Selected")
	m.storageMountBtn.AddCSSClass("suggested-action")
	m.storageMountBtn.ConnectClicked(func() { m.mountSelected() })
	modeBox.Append(m.storageMountBtn)
	box.Append(modeBox)

	m.tabStack.AddTitled(box, "storage", "Storage")
}

func (m *MediaOverlay) buildUploadTab() {
	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(8)

	pathBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	m.pathEntry = gtk.NewEntry()
	m.pathEntry.SetPlaceholderText("/path/to/image.iso")
	m.pathEntry.SetHExpand(true)
	m.browseBtn = gtk.NewButtonWithLabel("Browse")
	m.browseBtn.ConnectClicked(func() { m.browseFile() })
	pathBox.Append(m.pathEntry)
	pathBox.Append(m.browseBtn)
	box.Append(pathBox)

	modeBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	m.uploadModeCD = gtk.NewToggleButton()
	m.uploadModeCD.SetLabel("CD/DVD")
	m.uploadModeCD.SetActive(true)
	m.uploadModeDisk = gtk.NewToggleButton()
	m.uploadModeDisk.SetLabel("Disk")
	m.uploadModeDisk.SetGroup(m.uploadModeCD)
	modeBox.Append(m.uploadModeCD)
	modeBox.Append(m.uploadModeDisk)
	box.Append(modeBox)

	m.uploadBtn = gtk.NewButtonWithLabel("Start Upload")
	m.uploadBtn.AddCSSClass("suggested-action")
	m.uploadBtn.ConnectClicked(func() { m.startUpload() })
	box.Append(m.uploadBtn)

	m.progressBar = gtk.NewProgressBar()
	m.progressBar.SetVisible(false)
	box.Append(m.progressBar)

	m.progressLabel = gtk.NewLabel("")
	m.progressLabel.AddCSSClass("dim-label")
	m.progressLabel.SetXAlign(0)
	box.Append(m.progressLabel)

	m.tabStack.AddTitled(box, "upload", "Upload")
}

func (m *MediaOverlay) mountURL() {
	rawURL := strings.TrimSpace(m.urlEntry.Text())
	if rawURL == "" {
		return
	}
	if m.app.ctrl == nil {
		return
	}
	mode := virtualmedia.ModeCDROM
	if m.urlModeDisk.Active() {
		mode = virtualmedia.ModeDisk
	}
	_ = m.app.ctrl.MountMediaURL(rawURL, mode)
}

func (m *MediaOverlay) mountSelected() {
	row := m.storageList.SelectedRow()
	if row == nil || m.selectedFile == "" {
		return
	}
	if m.app.ctrl == nil {
		return
	}
	mode := virtualmedia.ModeCDROM
	if m.storageModeDisk.Active() {
		mode = virtualmedia.ModeDisk
	}
	_ = m.app.ctrl.MountStorageFile(m.selectedFile, mode)
}

func (m *MediaOverlay) browseFile() {
	// GTK4 file chooser — will be fully wired in Phase 6.
	// For now the user can type the path manually.
}

func (m *MediaOverlay) startUpload() {
	path := strings.TrimSpace(m.pathEntry.Text())
	if path == "" || m.app.ctrl == nil {
		return
	}
	m.progressBar.SetVisible(true)
	m.progressBar.SetFraction(0)
	m.uploadBtn.SetSensitive(false)

	go func() {
		err := m.app.ctrl.UploadStorageFile(path, func(p virtualmedia.UploadProgress) {
			// Progress callback runs on background goroutine.
			// TODO: use glib.IdleAdd to update progress bar on main thread.
			_ = p
		})
		_ = err
	}()
}

func (m *MediaOverlay) RefreshStorage() {
	tw, th := overlayTargetSize(m.app, 640, 520)
	m.Box.SetSizeRequest(tw, th)
	if m.app.ctrl == nil {
		return
	}
	files, err := m.app.ctrl.ListStorageFiles(context.Background())
	if err != nil {
		return
	}
	removeAllChildren(m.storageList)
	for _, f := range files {
		row := gtk.NewListBoxRow()
		box := gtk.NewBox(gtk.OrientationHorizontal, 8)
		box.SetMarginTop(4)
		box.SetMarginBottom(4)
		box.SetMarginStart(8)
		box.SetMarginEnd(8)

		nameLabel := gtk.NewLabel(f.Filename)
		nameLabel.SetHExpand(true)
		nameLabel.SetXAlign(0)

		sizeLabel := gtk.NewLabel(formatBytes(f.Size))
		sizeLabel.AddCSSClass("dim-label")

		delBtn := gtk.NewButtonFromIconName("user-trash-symbolic")
		delBtn.AddCSSClass("flat")
		filename := f.Filename
		delBtn.ConnectClicked(func() {
			if m.app.ctrl != nil {
				_ = m.app.ctrl.DeleteStorageFile(filename)
				m.RefreshStorage()
			}
		})

		box.Append(nameLabel)
		box.Append(sizeLabel)
		box.Append(delBtn)
		row.SetChild(box)
		m.storageList.Append(row)

		fn := f.Filename
		m.storageList.ConnectRowSelected(func(r *gtk.ListBoxRow) {
			if r == row {
				m.selectedFile = fn
			}
		})
	}
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

