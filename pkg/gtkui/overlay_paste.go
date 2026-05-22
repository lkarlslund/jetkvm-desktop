package gtkui

import (
	"strconv"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"golang.design/x/clipboard"

	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
)

// PasteOverlay lets the user send text to the remote via HID keyboard macros.
// The target's keyboard layout (from snap.KeyboardLayout) determines which
// characters can be sent; unsupported runes are flagged before send.
type PasteOverlay struct {
	Box *gtk.Box

	app         *Application
	layoutLabel *gtk.Label
	invalidLbl  *gtk.Label
	textView    *gtk.TextView
	delayEntry  *gtk.Entry
	statusLabel *gtk.Label
	sendBtn     *gtk.Button
	loadBtn     *gtk.Button
	cancelBtn   *gtk.Button
}

func NewPasteOverlay(app *Application) *PasteOverlay {
	p := &PasteOverlay{app: app}
	p.Box = gtk.NewBox(gtk.OrientationVertical, 8)
	p.Box.AddCSSClass("overlay-panel")
	p.Box.SetHAlign(gtk.AlignCenter)
	p.Box.SetVAlign(gtk.AlignCenter)

	title := gtk.NewLabel("Paste Text")
	title.AddCSSClass("title-3")
	title.SetXAlign(0)
	p.Box.Append(title)

	desc := gtk.NewLabel("Text is sent as HID keyboard macros over the target's keyboard layout. Unsupported characters are skipped.")
	desc.SetWrap(true)
	desc.AddCSSClass("dim-label")
	desc.SetXAlign(0)
	p.Box.Append(desc)

	p.layoutLabel = gtk.NewLabel("Target keyboard layout: —")
	p.layoutLabel.SetXAlign(0)
	p.layoutLabel.AddCSSClass("dim-label")
	p.Box.Append(p.layoutLabel)

	p.textView = gtk.NewTextView()
	p.textView.SetWrapMode(gtk.WrapWord)
	p.textView.SetMonospace(true)
	p.textView.SetVExpand(true)

	scroll := gtk.NewScrolledWindow()
	scroll.SetChild(p.textView)
	scroll.SetMinContentHeight(150)
	scroll.SetMaxContentHeight(300)
	p.Box.Append(scroll)

	p.textView.Buffer().ConnectChanged(func() {
		p.updatePreview()
	})

	p.invalidLbl = gtk.NewLabel("")
	p.invalidLbl.SetXAlign(0)
	p.invalidLbl.SetWrap(true)
	p.invalidLbl.AddCSSClass("dim-label")
	p.Box.Append(p.invalidLbl)

	delayRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	delayRow.SetHAlign(gtk.AlignStart)
	delayLbl := gtk.NewLabel("Delay between keystrokes (ms):")
	delayRow.Append(delayLbl)
	p.delayEntry = gtk.NewEntry()
	p.delayEntry.SetText("25")
	p.delayEntry.SetMaxWidthChars(6)
	p.delayEntry.SetWidthChars(6)
	p.delayEntry.SetInputPurpose(gtk.InputPurposeDigits)
	delayRow.Append(p.delayEntry)
	p.Box.Append(delayRow)

	p.statusLabel = gtk.NewLabel("")
	p.statusLabel.SetXAlign(0)
	p.statusLabel.AddCSSClass("dim-label")
	p.Box.Append(p.statusLabel)

	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	btnBox.SetHAlign(gtk.AlignEnd)

	p.loadBtn = gtk.NewButtonWithLabel("Load Clipboard")
	p.loadBtn.ConnectClicked(func() { p.loadClipboard() })

	p.cancelBtn = gtk.NewButtonWithLabel("Cancel")
	p.cancelBtn.ConnectClicked(func() {
		if p.app.ctrl != nil {
			p.app.ctrl.CancelPaste()
		}
		p.app.closeOverlay()
	})

	p.sendBtn = gtk.NewButtonWithLabel("Send")
	p.sendBtn.AddCSSClass("suggested-action")
	p.sendBtn.ConnectClicked(func() { p.send() })

	btnBox.Append(p.loadBtn)
	btnBox.Append(p.cancelBtn)
	btnBox.Append(p.sendBtn)
	p.Box.Append(btnBox)

	return p
}

// Refresh updates the keyboard layout label and refreshes the invalid-chars
// preview. Called when the paste overlay is opened.
func (p *PasteOverlay) Refresh() {
	tw, th := overlayTargetSize(p.app, 640, 520)
	p.Box.SetSizeRequest(tw, th)
	layout := "—"
	if p.app.ctrl != nil {
		if code := p.app.ctrl.Snapshot().KeyboardLayout; code != "" {
			layout = code
		}
	}
	p.layoutLabel.SetText("Target keyboard layout: " + layout)
	p.updatePreview()
}

func (p *PasteOverlay) updatePreview() {
	if p.app.ctrl == nil {
		p.invalidLbl.SetText("")
		return
	}
	buf := p.textView.Buffer()
	text := buf.Text(buf.StartIter(), buf.EndIter(), false)
	if text == "" {
		p.invalidLbl.SetText("")
		return
	}
	layout := p.app.ctrl.Snapshot().KeyboardLayout
	_, invalid := input.BuildPasteMacro(layout, text, 100)
	skipped := input.InvalidRunesString(invalid)
	if skipped == "" {
		p.invalidLbl.SetText("")
	} else {
		p.invalidLbl.SetText("Will skip: " + skipped)
	}
}

func (p *PasteOverlay) loadClipboard() {
	data := clipboard.Read(clipboard.FmtText)
	if len(data) > 0 {
		buf := p.textView.Buffer()
		buf.SetText(string(data))
	}
}

func (p *PasteOverlay) pasteDelay() uint16 {
	v, err := strconv.Atoi(p.delayEntry.Text())
	if err != nil || v < 1 {
		return 25
	}
	if v > 5000 {
		return 5000
	}
	return uint16(v)
}

func (p *PasteOverlay) send() {
	buf := p.textView.Buffer()
	text := buf.Text(buf.StartIter(), buf.EndIter(), false)
	if text == "" {
		return
	}
	if p.app.ctrl == nil {
		return
	}
	skipped, err := p.app.ctrl.ExecutePaste(text, p.pasteDelay())
	if err != nil {
		p.statusLabel.SetText("Error: " + err.Error())
		return
	}
	_ = skipped
	p.app.closeOverlay()
}
