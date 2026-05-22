package video

import "image"

// Decoder decodes H.264 Annex-B NAL units into images.
type Decoder interface {
	Decode(nalu []byte) (image.Image, error)
	Close() error
	// Name returns a human-readable label such as "openh264" or "vaapi".
	Name() string
}

// PackedYCbCr is an image.Image where each pixel is laid out as 4 bytes:
// Y, Cb, Cr, 0xff. Cb and Cr are nearest-neighbour upsampled to the full
// resolution so the layout looks like an RGBA buffer to Ebiten but renders
// to RGB via a YCbCr→RGB fragment shader on the GPU.
//
// Using this avoids ~30 MB/s of CPU work that swscale would otherwise do
// to produce real RGBA frames.
type PackedYCbCr struct {
	*image.RGBA
}

