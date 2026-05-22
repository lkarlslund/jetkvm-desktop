package video

import "log"

// NewDecoder returns the best available H.264 decoder.
// It tries FFmpeg (with VA-API hardware acceleration) first and falls back to
// the bundled OpenH264 software decoder.
func NewDecoder() (Decoder, error) {
	if dec, err := newFFmpegDecoder(); err == nil {
		log.Printf("[video] using decoder: %s", dec.Name())
		return dec, nil
	}
	dec, err := newOpenH264Decoder()
	if err != nil {
		return nil, err
	}
	log.Printf("[video] using decoder: %s", dec.Name())
	return dec, nil
}
