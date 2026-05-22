package gtkui

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

// SerialOverlay shows the serial console output and accepts keyboard input.
type SerialOverlay struct {
	Box *gtk.Box

	app         *Application
	textView    *gtk.TextView
	statusLabel *gtk.Label
	lastLen     int
}

func NewSerialOverlay(app *Application) *SerialOverlay {
	s := &SerialOverlay{app: app}
	s.Box = gtk.NewBox(gtk.OrientationVertical, 8)
	s.Box.AddCSSClass("overlay-panel")
	s.Box.SetHAlign(gtk.AlignCenter)
	s.Box.SetVAlign(gtk.AlignCenter)

	header := gtk.NewBox(gtk.OrientationHorizontal, 8)
	title := gtk.NewLabel("Serial Console")
	title.AddCSSClass("title-3")
	title.SetHExpand(true)
	title.SetXAlign(0)

	closeBtn := gtk.NewButtonFromIconName("window-close-symbolic")
	closeBtn.AddCSSClass("flat")
	closeBtn.ConnectClicked(func() { app.closeOverlay() })

	header.Append(title)
	header.Append(closeBtn)
	s.Box.Append(header)

	s.statusLabel = gtk.NewLabel("")
	s.statusLabel.AddCSSClass("dim-label")
	s.statusLabel.SetXAlign(0)
	s.Box.Append(s.statusLabel)

	s.textView = gtk.NewTextView()
	s.textView.SetEditable(false)
	s.textView.SetMonospace(true)
	s.textView.SetWrapMode(gtk.WrapWord)
	s.textView.SetVExpand(true)

	scroll := gtk.NewScrolledWindow()
	scroll.SetChild(s.textView)
	scroll.SetMinContentHeight(250)
	s.Box.Append(scroll)

	hint := gtk.NewLabel("Type to send to serial port. Press Esc to close.")
	hint.AddCSSClass("dim-label")
	hint.SetXAlign(0)
	s.Box.Append(hint)

	// Keyboard input for serial
	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval, _ uint, _ gdk.ModifierType) bool {
		if app.ctrl == nil {
			return false
		}
		switch keyval {
		case 0xff0d, 0xff8d: // Return, KP_Enter
			_ = app.ctrl.SendSerialTerminator()
			return true
		case 0xff08: // BackSpace
			_ = app.ctrl.SendSerialRaw("\x7f")
			return true
		case 0xff09: // Tab
			_ = app.ctrl.SendSerialRaw("\t")
			return true
		case 0xff1b: // Escape
			app.closeOverlay()
			return true
		default:
			if keyval >= 0x20 && keyval <= 0x7e {
				_ = app.ctrl.SendSerialText(string(rune(keyval)))
				return true
			}
		}
		return false
	})
	s.Box.AddController(keyCtrl)

	return s
}

func (s *SerialOverlay) Update(snap session.Snapshot) {
	tw, th := overlayTargetSize(s.app, 720, 540)
	s.Box.SetSizeRequest(tw, th)
	if !snap.SerialConsoleReady {
		s.statusLabel.SetText("Serial console not ready")
		return
	}
	s.statusLabel.SetText("Connected")

	buf := snap.SerialConsoleBuffer
	if len(buf) != s.lastLen {
		s.lastLen = len(buf)
		s.textView.Buffer().SetText(buf)
	}
}
