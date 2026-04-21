package openh264

import (
	"image"
	"io"
	"testing"
)

func TestEncoderCreate(t *testing.T) {
	params := NewEncoderParams()
	params.Width = 640
	params.Height = 480

	encoder, err := NewEncoder(params)
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}
	if encoder == nil {
		t.Fatal("encoder is nil")
	}
	if err := encoder.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestEncoderCloseTwice(t *testing.T) {
	params := NewEncoderParams()
	params.Width = 640
	params.Height = 480

	encoder, err := NewEncoder(params)
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}

	if err := encoder.Close(); err != nil {
		t.Fatalf("First Close failed: %v", err)
	}

	if err := encoder.Close(); err != nil {
		t.Fatalf("Second Close failed: %v", err)
	}
}

func TestEncoderEncodeAfterClose(t *testing.T) {
	params := NewEncoderParams()
	params.Width = 256
	params.Height = 144

	encoder, err := NewEncoder(params)
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}

	if err := encoder.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	img := image.NewYCbCr(
		image.Rect(0, 0, 256, 144),
		image.YCbCrSubsampleRatio420,
	)
	_, err = encoder.Encode(img)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestEncoderSimpleEncode(t *testing.T) {
	params := NewEncoderParams()
	params.Width = 256
	params.Height = 144

	encoder, err := NewEncoder(params)
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}
	defer encoder.Close()

	img := image.NewYCbCr(
		image.Rect(0, 0, 256, 144),
		image.YCbCrSubsampleRatio420,
	)

	// Fill with test pattern
	for y := 0; y < 144; y++ {
		for x := 0; x < 256; x++ {
			img.Y[y*img.YStride+x] = uint8((x + y) % 256)
		}
	}
	for y := 0; y < 72; y++ {
		for x := 0; x < 128; x++ {
			img.Cb[y*img.CStride+x] = 128
			img.Cr[y*img.CStride+x] = 128
		}
	}

	data, err := encoder.Encode(img)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("encoded data is empty")
	}
	t.Logf("Encoded frame size: %d bytes", len(data))
}

func TestEncoderForceKeyFrame(t *testing.T) {
	params := NewEncoderParams()
	params.Width = 256
	params.Height = 144

	encoder, err := NewEncoder(params)
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}
	defer encoder.Close()

	if err := encoder.ForceKeyFrame(); err != nil {
		t.Fatalf("ForceKeyFrame failed: %v", err)
	}
}

func TestEncoderSetBitRate(t *testing.T) {
	params := NewEncoderParams()
	params.Width = 256
	params.Height = 144

	encoder, err := NewEncoder(params)
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}
	defer encoder.Close()

	if err := encoder.SetBitRate(500000); err != nil {
		t.Fatalf("SetBitRate failed: %v", err)
	}
}
