//go:build !ffmpeg

package video

import "errors"

func newFFmpegDecoder() (Decoder, error) {
	return nil, errors.New("ffmpeg decoder not compiled in (build with -tags ffmpeg)")
}
