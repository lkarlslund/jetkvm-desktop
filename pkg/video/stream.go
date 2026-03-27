package video

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"runtime"
	"sync"
	"time"

	openh264 "github.com/Azunyan1111/openh264-go"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"
)

type Frame struct {
	Image image.Image
	At    time.Time
}

type Stream struct {
	mu         sync.RWMutex
	latest     *Frame
	frameCh    chan Frame
	closeOnce  sync.Once
	cancelFunc context.CancelFunc
	onClose    func() error
}

func NewStream() *Stream {
	return &Stream{frameCh: make(chan Frame, 4)}
}

func (s *Stream) Latest() *Frame {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latest
}

func (s *Stream) publish(frame Frame) {
	s.mu.Lock()
	s.latest = &frame
	s.mu.Unlock()

	select {
	case s.frameCh <- frame:
	default:
	}
}

func (s *Stream) Frames() <-chan Frame {
	return s.frameCh
}

func (s *Stream) Close() {
	s.closeOnce.Do(func() {
		if s.cancelFunc != nil {
			s.cancelFunc()
		}
		if s.onClose != nil {
			_ = s.onClose()
		}
		close(s.frameCh)
	})
}

func AttachRemoteTrack(parent context.Context, track *webrtc.TrackRemote) (*Stream, error) {
	if track.Codec().MimeType != webrtc.MimeTypeH264 {
		return nil, fmt.Errorf("unsupported codec %s", track.Codec().MimeType)
	}

	ctx, cancel := context.WithCancel(parent)
	stream := NewStream()
	stream.cancelFunc = cancel

	decoder, err := openh264.NewDecoder(bytes.NewReader(nil))
	if err != nil {
		cancel()
		return nil, err
	}
	stream.onClose = decoder.Close

	go func() {
		defer stream.Close()

		// Screen-content H.264 keyframes can span far more than 32 RTP packets.
		// A too-small samplebuilder buffer drops fragmented access units before the
		// decoder ever sees a complete frame, which showed up as intermittent
		// "no first frame" failures on slower CI runners.
		sb := samplebuilder.New(512, &codecs.H264Packet{}, track.Codec().ClockRate)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			pkt, _, readErr := track.ReadRTP()
			if readErr != nil {
				return
			}
			sb.Push(pkt)
			for {
				sample := sb.Pop()
				if sample == nil {
					break
				}
				payload := ensureAnnexB(sample.Data)
				if len(payload) == 0 {
					continue
				}
				img, err := decoder.Decode(payload)
				if err != nil {
					return
				}
				if img != nil {
					stream.publish(Frame{Image: img, At: time.Now()})
				}
			}
		}
	}()

	return stream, nil
}

func StartTestPattern(ctx context.Context, width, height, fps int, track *webrtc.TrackLocalStaticSample) error {
	params := openh264.NewEncoderParams()
	params.Width = width
	params.Height = height
	params.BitRate = width * height * 4
	params.MaxFrameRate = float32(fps)
	params.UsageType = openh264.ScreenContentRealTime
	params.EnableFrameSkip = false
	params.IntraPeriod = uint(fps * 2)

	encoder, err := openh264.NewEncoder(params)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(time.Second / time.Duration(fps))
	go func() {
		defer ticker.Stop()
		defer closeEncoder(encoder)

		frameIndex := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if frameIndex%(fps*2) == 0 {
					_ = encoder.ForceKeyFrame()
				}
				img := newPatternFrame(width, height, frameIndex)
				payload, err := encoder.Encode(img)
				if err != nil {
					return
				}
				if len(payload) == 0 {
					frameIndex++
					continue
				}
				if err := track.WriteSample(media.Sample{
					Data:     payload,
					Duration: time.Second / time.Duration(fps),
				}); err != nil {
					return
				}
				frameIndex++
			}
		}
	}()

	return nil
}

func closeEncoder(encoder *openh264.Encoder) {
	// openh264-go encoder teardown can block indefinitely on Windows in CI.
	// The process is short-lived in tests and the OS will reclaim resources.
	if runtime.GOOS == "windows" {
		return
	}
	_ = encoder.Close()
}

func newPatternFrame(width, height, frameIndex int) *image.YCbCr {
	img := image.NewYCbCr(image.Rect(0, 0, width, height), image.YCbCrSubsampleRatio420)
	phase := frameIndex % 255

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			yy := uint8((x + y + phase) % 256)
			img.Y[y*img.YStride+x] = yy
		}
	}

	for y := 0; y < height/2; y++ {
		for x := 0; x < width/2; x++ {
			img.Cb[y*img.CStride+x] = uint8((x*2 + phase*3) % 256)
			img.Cr[y*img.CStride+x] = uint8((y*2 + phase*5) % 256)
		}
	}

	drawBox(img, 20+(frameIndex*7)%(max(1, width-120)), 20+(frameIndex*5)%(max(1, height-80)), 100, 60, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	return img
}

func drawBox(img *image.YCbCr, x0, y0, w, h int, c color.Color) {
	ycbcr := color.YCbCrModel.Convert(c).(color.YCbCr)
	for y := max(0, y0); y < minInt(img.Rect.Dy(), y0+h); y++ {
		for x := max(0, x0); x < minInt(img.Rect.Dx(), x0+w); x++ {
			img.Y[y*img.YStride+x] = ycbcr.Y
			img.Cb[(y/2)*img.CStride+(x/2)] = ycbcr.Cb
			img.Cr[(y/2)*img.CStride+(x/2)] = ycbcr.Cr
		}
	}
}

func ensureAnnexB(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	if bytes.HasPrefix(data, []byte{0x00, 0x00, 0x00, 0x01}) || bytes.HasPrefix(data, []byte{0x00, 0x00, 0x01}) {
		return data
	}
	return append([]byte{0x00, 0x00, 0x00, 0x01}, data...)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
