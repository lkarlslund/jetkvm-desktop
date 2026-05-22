package gtkui

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/discovery"
	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

// Launcher shows the device browser / manual connect / password screens.
type Launcher struct {
	Stack *gtk.Stack

	app     *Application
	scanner *discovery.Scanner
	devices []discovery.Device
	prefs   *Preferences

	// Browse page widgets
	browsePage     *gtk.Box
	deviceList     *gtk.ListBox
	deviceFrame    *gtk.Frame
	recentList     *gtk.ListBox
	urlEntry       *gtk.Entry
	connectBtn     *gtk.Button
	errorLabel     *gtk.Label
	scanLabel      *gtk.Label
	scanSpinner    *gtk.Spinner
	connectingBox  *gtk.Box
	connectingLbl  *gtk.Label
	connectSpinner *gtk.Spinner

	// URL lookup tables keyed by row index
	deviceURLs []string
	recentURLs []string

	// Password page widgets
	passwordPage   *gtk.Box
	targetLabel    *gtk.Label
	passwordEntry  *gtk.PasswordEntry
	passConnBtn    *gtk.Button
	passBackBtn    *gtk.Button
	passErrorLabel *gtk.Label

	pendingURL string
}

func NewLauncher(app *Application, prefs *Preferences) *Launcher {
	l := &Launcher{
		Stack: gtk.NewStack(),
		app:   app,
		prefs: prefs,
	}
	l.buildBrowsePage()
	l.buildPasswordPage()

	l.Stack.AddNamed(l.browsePage, "browse")
	l.Stack.AddNamed(l.passwordPage, "password")
	l.Stack.SetVisibleChildName("browse")

	l.scanner = discovery.NewScanner()
	l.scanner.Start(context.Background())

	glib.TimeoutAdd(500, func() bool {
		l.drainDiscovery()
		return true
	})

	l.refreshRecents()
	return l
}

func (l *Launcher) buildBrowsePage() {
	l.browsePage = gtk.NewBox(gtk.OrientationVertical, 12)
	l.browsePage.SetMarginTop(24)
	l.browsePage.SetMarginBottom(24)
	l.browsePage.SetMarginStart(24)
	l.browsePage.SetMarginEnd(24)
	l.browsePage.SetVAlign(gtk.AlignCenter)
	l.browsePage.SetHAlign(gtk.AlignCenter)

	title := gtk.NewLabel("JetKVM")
	title.AddCSSClass("title-1")
	l.browsePage.Append(title)

	subtitle := gtk.NewLabel("Available devices on your local network")
	subtitle.AddCSSClass("dim-label")
	l.browsePage.Append(subtitle)

	// Discovery list
	l.scanLabel = gtk.NewLabel("Scanning local subnets…")
	l.scanLabel.AddCSSClass("dim-label")
	l.scanLabel.SetXAlign(0)

	l.deviceList = gtk.NewListBox()
	l.deviceList.SetSelectionMode(gtk.SelectionNone)
	l.deviceList.AddCSSClass("boxed-list")
	l.deviceList.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		idx := row.Index()
		if idx >= 0 && idx < len(l.deviceURLs) {
			url := l.deviceURLs[idx]
			glib.IdleAdd(func() { l.connectTo(url) })
		}
	})

	scanRow := gtk.NewBox(gtk.OrientationHorizontal, 6)
	scanRow.SetHAlign(gtk.AlignStart)
	l.scanSpinner = gtk.NewSpinner()
	l.scanSpinner.Start()
	scanRow.Append(l.scanSpinner)
	scanRow.Append(l.scanLabel)

	l.deviceFrame = gtk.NewFrame("")
	l.deviceFrame.SetChild(l.deviceList)
	l.deviceFrame.SetVisible(false)
	l.browsePage.Append(scanRow)
	l.browsePage.Append(l.deviceFrame)

	// Recents
	recentsLabel := gtk.NewLabel("Recently connected")
	recentsLabel.AddCSSClass("dim-label")
	recentsLabel.SetXAlign(0)
	l.browsePage.Append(recentsLabel)

	l.recentList = gtk.NewListBox()
	l.recentList.SetSelectionMode(gtk.SelectionNone)
	l.recentList.AddCSSClass("boxed-list")
	l.recentList.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		idx := row.Index()
		if idx >= 0 && idx < len(l.recentURLs) {
			url := l.recentURLs[idx]
			glib.IdleAdd(func() { l.connectTo(url) })
		}
	})

	recentFrame := gtk.NewFrame("")
	recentFrame.SetChild(l.recentList)
	l.browsePage.Append(recentFrame)

	// Manual connect
	manualBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	l.urlEntry = gtk.NewEntry()
	l.urlEntry.SetPlaceholderText("jetkvm.local or 192.168.1.50")
	l.urlEntry.SetHExpand(true)
	l.urlEntry.ConnectActivate(func() { l.onConnect() })

	l.connectBtn = gtk.NewButtonWithLabel("Connect")
	l.connectBtn.AddCSSClass("suggested-action")
	l.connectBtn.ConnectClicked(func() { l.onConnect() })

	manualBox.Append(l.urlEntry)
	manualBox.Append(l.connectBtn)
	l.browsePage.Append(manualBox)

	l.errorLabel = gtk.NewLabel("")
	l.errorLabel.AddCSSClass("error")
	l.errorLabel.SetXAlign(0)
	l.browsePage.Append(l.errorLabel)

	l.connectingBox = gtk.NewBox(gtk.OrientationHorizontal, 8)
	l.connectingBox.SetHAlign(gtk.AlignCenter)
	l.connectingBox.SetVisible(false)
	l.connectSpinner = gtk.NewSpinner()
	l.connectingLbl = gtk.NewLabel("Connecting…")
	l.connectingLbl.AddCSSClass("dim-label")
	l.connectingBox.Append(l.connectSpinner)
	l.connectingBox.Append(l.connectingLbl)
	l.browsePage.Append(l.connectingBox)
}

func (l *Launcher) buildPasswordPage() {
	l.passwordPage = gtk.NewBox(gtk.OrientationVertical, 12)
	l.passwordPage.SetMarginTop(24)
	l.passwordPage.SetMarginBottom(24)
	l.passwordPage.SetMarginStart(24)
	l.passwordPage.SetMarginEnd(24)
	l.passwordPage.SetVAlign(gtk.AlignCenter)
	l.passwordPage.SetHAlign(gtk.AlignCenter)

	title := gtk.NewLabel("Password Required")
	title.AddCSSClass("title-2")
	l.passwordPage.Append(title)

	l.targetLabel = gtk.NewLabel("")
	l.targetLabel.AddCSSClass("dim-label")
	l.passwordPage.Append(l.targetLabel)

	l.passwordEntry = gtk.NewPasswordEntry()
	l.passwordEntry.SetShowPeekIcon(true)
	l.passwordPage.Append(l.passwordEntry)

	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	btnBox.SetHAlign(gtk.AlignCenter)

	l.passBackBtn = gtk.NewButtonWithLabel("Back")
	l.passBackBtn.ConnectClicked(func() {
		l.Stack.SetVisibleChildName("browse")
	})

	l.passConnBtn = gtk.NewButtonWithLabel("Connect")
	l.passConnBtn.AddCSSClass("suggested-action")
	l.passConnBtn.ConnectClicked(func() { l.onPasswordConnect() })

	btnBox.Append(l.passBackBtn)
	btnBox.Append(l.passConnBtn)
	l.passwordPage.Append(btnBox)

	l.passErrorLabel = gtk.NewLabel("")
	l.passErrorLabel.AddCSSClass("error")
	l.passwordPage.Append(l.passErrorLabel)
}

func (l *Launcher) setConnecting(active bool) {
	l.deviceList.SetSensitive(!active)
	l.recentList.SetSensitive(!active)
	l.urlEntry.SetSensitive(!active)
	l.connectBtn.SetSensitive(!active)
	l.connectingBox.SetVisible(active)
	if active {
		l.connectSpinner.Start()
		l.errorLabel.SetText("")
	} else {
		l.connectSpinner.Stop()
	}
}

func (l *Launcher) onConnect() {
	host := strings.TrimSpace(l.urlEntry.Text())
	if host == "" {
		return
	}
	baseURL := normalizeURL(host)
	if !isValidHost(host) {
		l.errorLabel.SetText("Invalid host: " + host)
		return
	}
	l.errorLabel.SetText("")
	l.connectTo(baseURL)
}

func (l *Launcher) connectTo(baseURL string) {
	l.prefs.addRecent(baseURL, "")
	_ = savePrefs(*l.prefs)

	l.pendingURL = baseURL
	l.setConnecting(true)
	l.connectingLbl.SetText("Connecting to " + baseURL + "…")
	l.app.startSession(baseURL, "")

	glib.IdleAdd(func() { l.refreshRecents() })

	glib.TimeoutAdd(200, func() bool {
		if l.app.ctrl == nil {
			l.setConnecting(false)
			return false
		}
		snap := l.app.ctrl.Snapshot()
		switch snap.Phase {
		case session.PhaseAuthFailed:
			l.setConnecting(false)
			l.targetLabel.SetText(baseURL)
			l.passErrorLabel.SetText("Authentication required")
			l.Stack.SetVisibleChildName("password")
			return false
		case session.PhaseConnected:
			l.setConnecting(false)
			l.app.showSession()
			return false
		case session.PhaseFatal:
			l.setConnecting(false)
			l.errorLabel.SetText(snap.LastError)
			return false
		}
		return true
	})
}

func (l *Launcher) onPasswordConnect() {
	buf := l.passwordEntry.ObjectProperty("text")
	password := ""
	if s, ok := buf.(string); ok {
		password = s
	}
	if password == "" {
		return
	}
	l.passErrorLabel.SetText("")
	l.passConnBtn.SetSensitive(false)
	l.passBackBtn.SetSensitive(false)
	l.passwordEntry.SetSensitive(false)
	l.app.startSession(l.pendingURL, password)

	glib.TimeoutAdd(200, func() bool {
		if l.app.ctrl == nil {
			l.passConnBtn.SetSensitive(true)
			l.passBackBtn.SetSensitive(true)
			l.passwordEntry.SetSensitive(true)
			return false
		}
		snap := l.app.ctrl.Snapshot()
		switch snap.Phase {
		case session.PhaseAuthFailed:
			l.passConnBtn.SetSensitive(true)
			l.passBackBtn.SetSensitive(true)
			l.passwordEntry.SetSensitive(true)
			l.passErrorLabel.SetText("Incorrect password")
			return false
		case session.PhaseConnected:
			l.app.showSession()
			return false
		case session.PhaseFatal:
			l.passConnBtn.SetSensitive(true)
			l.passBackBtn.SetSensitive(true)
			l.passwordEntry.SetSensitive(true)
			l.passErrorLabel.SetText(snap.LastError)
			return false
		}
		return true
	})
}

func (l *Launcher) drainDiscovery() {
	changed := false
	for {
		select {
		case dev := <-l.scanner.Updates():
			l.mergeDevice(dev)
			changed = true
		default:
			if changed {
				l.rebuildDeviceList()
			}
			return
		}
	}
}

func (l *Launcher) mergeDevice(dev discovery.Device) {
	for i, d := range l.devices {
		if d.BaseURL == dev.BaseURL {
			l.devices[i] = dev
			return
		}
	}
	l.devices = append(l.devices, dev)
}

func (l *Launcher) rebuildDeviceList() {
	removeAllChildren(l.deviceList)
	l.deviceURLs = nil

	if len(l.devices) == 0 {
		l.scanLabel.SetText("Scanning local subnets…")
		l.scanSpinner.Start()
		l.scanSpinner.SetVisible(true)
		l.deviceFrame.SetVisible(false)
		return
	}
	l.scanLabel.SetText(fmt.Sprintf("%d device(s) found", len(l.devices)))
	l.scanSpinner.Stop()
	l.scanSpinner.SetVisible(false)
	l.deviceFrame.SetVisible(true)

	for _, dev := range l.devices {
		l.deviceURLs = append(l.deviceURLs, dev.BaseURL)

		box := gtk.NewBox(gtk.OrientationHorizontal, 8)
		box.SetMarginTop(6)
		box.SetMarginBottom(6)
		box.SetMarginStart(8)
		box.SetMarginEnd(8)

		nameLabel := gtk.NewLabel(dev.Name)
		nameLabel.SetHExpand(true)
		nameLabel.SetXAlign(0)
		box.Append(nameLabel)

		urlLabel := gtk.NewLabel(dev.BaseURL)
		urlLabel.AddCSSClass("dim-label")
		box.Append(urlLabel)

		status := "Configured"
		if !dev.IsSetup {
			status = "Needs setup"
		}
		statusLabel := gtk.NewLabel(status)
		statusLabel.AddCSSClass("dim-label")
		box.Append(statusLabel)

		row := gtk.NewListBoxRow()
		row.SetChild(box)
		row.SetActivatable(true)
		l.deviceList.Append(row)
	}
}

func (l *Launcher) refreshRecents() {
	removeAllChildren(l.recentList)
	l.recentURLs = nil

	maxShow := 3
	if len(l.prefs.RecentConnections) < maxShow {
		maxShow = len(l.prefs.RecentConnections)
	}

	for i := 0; i < maxShow; i++ {
		rc := l.prefs.RecentConnections[i]
		l.recentURLs = append(l.recentURLs, rc.URL)

		box := gtk.NewBox(gtk.OrientationHorizontal, 8)
		box.SetMarginTop(6)
		box.SetMarginBottom(6)
		box.SetMarginStart(8)
		box.SetMarginEnd(8)

		label := rc.URL
		if rc.Name != "" {
			label = rc.Name
		}
		nameLabel := gtk.NewLabel(label)
		nameLabel.SetHExpand(true)
		nameLabel.SetXAlign(0)
		box.Append(nameLabel)

		age := time.Since(rc.ConnectedAt).Truncate(time.Minute)
		ageLabel := gtk.NewLabel(formatAge(age))
		ageLabel.AddCSSClass("dim-label")
		box.Append(ageLabel)

		deleteBtn := gtk.NewButtonFromIconName("window-close-symbolic")
		deleteBtn.AddCSSClass("flat")
		deleteURL := rc.URL
		deleteBtn.ConnectClicked(func() {
			l.prefs.removeRecent(deleteURL)
			_ = savePrefs(*l.prefs)
			glib.IdleAdd(func() { l.refreshRecents() })
		})
		box.Append(deleteBtn)

		row := gtk.NewListBoxRow()
		row.SetChild(box)
		row.SetActivatable(true)
		l.recentList.Append(row)
	}
}

func (l *Launcher) Stop() {
	l.scanner.Stop()
}

func normalizeURL(host string) string {
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		return host
	}
	return "https://" + host
}

func isValidHost(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	u, err := url.Parse(normalizeURL(s))
	if err != nil {
		return false
	}
	return u.Hostname() != ""
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
}

func removeAllChildren(list *gtk.ListBox) {
	for {
		row := list.RowAtIndex(0)
		if row == nil {
			break
		}
		list.Remove(row)
	}
}
