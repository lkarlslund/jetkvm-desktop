package gtkui

import (
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/client"
	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

// StatsOverlay shows live connection metrics.
type StatsOverlay struct {
	Box *gtk.Box

	app    *Application
	labels map[string]*gtk.Label
}

func NewStatsOverlay(app *Application) *StatsOverlay {
	s := &StatsOverlay{
		app:    app,
		labels: make(map[string]*gtk.Label),
	}
	s.Box = gtk.NewBox(gtk.OrientationVertical, 4)
	s.Box.AddCSSClass("overlay-panel")
	s.Box.SetHAlign(gtk.AlignCenter)
	s.Box.SetVAlign(gtk.AlignCenter)

	title := gtk.NewLabel("Connection Stats")
	title.AddCSSClass("title-3")
	title.SetXAlign(0)
	s.Box.Append(title)

	fields := []string{
		"Signaling", "RTC State", "HID", "Video",
		"Resolution", "Quality", "Frame Age",
		"Bitrate", "Decode FPS", "Jitter", "RTT",
		"Packets Lost", "Error",
	}
	for _, f := range fields {
		row := gtk.NewBox(gtk.OrientationHorizontal, 8)
		nameLabel := gtk.NewLabel(f)
		nameLabel.AddCSSClass("dim-label")
		nameLabel.SetXAlign(0)
		nameLabel.SetHExpand(true)

		valLabel := gtk.NewLabel("—")
		valLabel.SetXAlign(1)

		row.Append(nameLabel)
		row.Append(valLabel)
		s.Box.Append(row)
		s.labels[f] = valLabel
	}

	return s
}

func (s *StatsOverlay) Update(snap session.Snapshot, stats client.StatsSnapshot) {
	tw, th := overlayTargetSize(s.app, 560, 440)
	s.Box.SetSizeRequest(tw, th)
	set := func(key, val string) {
		if l, ok := s.labels[key]; ok {
			l.SetText(val)
		}
	}

	set("Signaling", string(stats.SignalingMode))
	set("RTC State", stats.RTCState.String())
	set("HID", boolReady(stats.HIDReady))
	set("Video", boolReady(stats.VideoReady))
	set("Resolution", fmt.Sprintf("%dx%d", stats.FrameWidth, stats.FrameHeight))
	set("Quality", fmt.Sprintf("%.0f%%", snap.Quality*100))
	set("Bitrate", fmt.Sprintf("%.0f kbps", stats.BitrateKbps))
	set("Decode FPS", fmt.Sprintf("%.1f", stats.FramesPerSecond))
	set("Jitter", fmt.Sprintf("%.1f ms", stats.JitterMs))
	set("RTT", fmt.Sprintf("%.1f ms", stats.RoundTripMs))
	set("Packets Lost", fmt.Sprintf("%d", stats.PacketsLost))
	set("Error", stats.LastError)
}

func boolReady(b bool) string {
	if b {
		return "Ready"
	}
	return "Pending"
}
