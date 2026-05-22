package video

import (
	"bytes"
	"image"

	openh264 "github.com/Azunyan1111/openh264-go"
)

type openh264Decoder struct {
	dec *openh264.Decoder
}

func newOpenH264Decoder() (Decoder, error) {
	dec, err := openh264.NewDecoder(bytes.NewReader(nil))
	if err != nil {
		return nil, err
	}
	return &openh264Decoder{dec: dec}, nil
}

func (d *openh264Decoder) Decode(nalu []byte) (image.Image, error) {
	return d.dec.Decode(nalu)
}

func (d *openh264Decoder) Close() error { return d.dec.Close() }
func (d *openh264Decoder) Name() string { return "openh264" }
