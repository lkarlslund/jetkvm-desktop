package openh264

import (
	"bytes"
	"image"
	"io"
	"math"
	"sync"
	"testing"
)

func TestDecoderCreate(t *testing.T) {
	r := bytes.NewReader([]byte{})
	decoder, err := NewDecoder(r)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	if decoder == nil {
		t.Fatal("decoder is nil")
	}
	if err := decoder.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestDecoderCloseTwice(t *testing.T) {
	r := bytes.NewReader([]byte{})
	decoder, err := NewDecoder(r)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}

	if err := decoder.Close(); err != nil {
		t.Fatalf("First Close failed: %v", err)
	}

	if err := decoder.Close(); err != nil {
		t.Fatalf("Second Close failed: %v", err)
	}
}

func TestDecoderReadAfterClose(t *testing.T) {
	r := bytes.NewReader([]byte{})
	decoder, err := NewDecoder(r)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}

	if err := decoder.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	_, err = decoder.Read()
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestH264EncodeDecode(t *testing.T) {
	width, height := 320, 240
	reader, writer := io.Pipe()

	// Create decoder
	decoder, err := NewDecoder(reader)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer decoder.Close()

	// Create encoder
	params := NewEncoderParams()
	params.Width = width
	params.Height = height
	params.MaxFrameRate = 30.0
	params.IntraPeriod = 1 // All frames are IDR to minimize buffering

	encoder, err := NewEncoder(params)
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}
	defer encoder.Close()

	totalFrames := 10
	var wg sync.WaitGroup
	wg.Add(1)

	decodedFrames := 0

	// Decoder goroutine
	go func() {
		defer wg.Done()
		for {
			img, err := decoder.Read()
			if err == io.EOF {
				return
			}
			if err != nil {
				t.Errorf("decoder read error: %v", err)
				return
			}
			bounds := img.Bounds()
			if bounds.Dx() != width || bounds.Dy() != height {
				t.Errorf("unexpected frame size: got %dx%d, want %dx%d",
					bounds.Dx(), bounds.Dy(), width, height)
			}
			decodedFrames++
		}
	}()

	// Encode and send frames
	for i := 0; i < totalFrames; i++ {
		img := image.NewYCbCr(
			image.Rect(0, 0, width, height),
			image.YCbCrSubsampleRatio420,
		)
		// Generate test pattern
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				img.Y[y*img.YStride+x] = uint8((x + y + i*10) % 256)
			}
		}
		for y := 0; y < height/2; y++ {
			for x := 0; x < width/2; x++ {
				img.Cb[y*img.CStride+x] = 128
				img.Cr[y*img.CStride+x] = 128
			}
		}

		data, err := encoder.Encode(img)
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatalf("writer error: %v", err)
		}
	}
	writer.Close()
	wg.Wait()

	if decodedFrames != totalFrames {
		t.Errorf("frame count mismatch: got %d, want %d", decodedFrames, totalFrames)
	}
}

func TestDecodedFrameContent(t *testing.T) {
	width, height := 320, 240
	reader, writer := io.Pipe()

	// Create decoder
	decoder, err := NewDecoder(reader)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer decoder.Close()

	// Create encoder with high bitrate for quality
	params := NewEncoderParams()
	params.Width = width
	params.Height = height
	params.MaxFrameRate = 30.0
	params.BitRate = 2_000_000
	params.IntraPeriod = 1

	encoder, err := NewEncoder(params)
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}
	defer encoder.Close()

	var inputImages []*image.YCbCr
	var outputImages []*image.YCbCr
	totalFrames := 5
	var wg sync.WaitGroup
	wg.Add(1)

	// Decoder goroutine
	go func() {
		defer wg.Done()
		for {
			img, err := decoder.Read()
			if err == io.EOF {
				return
			}
			if err != nil {
				t.Errorf("decoder read error: %v", err)
				return
			}
			outputImages = append(outputImages, copyYCbCr(img))
		}
	}()

	// Encode and send frames
	for i := 0; i < totalFrames; i++ {
		img := image.NewYCbCr(
			image.Rect(0, 0, width, height),
			image.YCbCrSubsampleRatio420,
		)
		// Generate gradient pattern
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				img.Y[y*img.YStride+x] = uint8((x*2 + y) % 256)
			}
		}
		for y := 0; y < height/2; y++ {
			for x := 0; x < width/2; x++ {
				img.Cb[y*img.CStride+x] = uint8((x + 64) % 256)
				img.Cr[y*img.CStride+x] = uint8((y + 64) % 256)
			}
		}
		inputImages = append(inputImages, copyYCbCr(img))

		data, err := encoder.Encode(img)
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatalf("writer error: %v", err)
		}
	}
	writer.Close()
	wg.Wait()

	// PSNR verification
	if len(outputImages) != len(inputImages) {
		t.Fatalf("frame count mismatch: got %d, want %d", len(outputImages), len(inputImages))
	}

	minPSNR := 30.0 // Expect at least 30dB for H.264
	for i := range outputImages {
		psnr := calculatePSNR(inputImages[i], outputImages[i])
		t.Logf("Frame %d PSNR: %.2f dB", i, psnr)
		if psnr < minPSNR {
			t.Errorf("Frame %d PSNR too low: %.2f dB < %.2f dB", i, psnr, minPSNR)
		}
	}
}

func TestDecoderDecodeMethod(t *testing.T) {
	width, height := 320, 240

	// Create encoder
	params := NewEncoderParams()
	params.Width = width
	params.Height = height
	params.MaxFrameRate = 30.0
	params.IntraPeriod = 1

	encoder, err := NewEncoder(params)
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}
	defer encoder.Close()

	// Create decoder without reader (will use Decode method)
	decoder, err := NewDecoder(bytes.NewReader([]byte{}))
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer decoder.Close()

	// Encode a frame
	img := image.NewYCbCr(
		image.Rect(0, 0, width, height),
		image.YCbCrSubsampleRatio420,
	)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Y[y*img.YStride+x] = uint8((x + y) % 256)
		}
	}
	for y := 0; y < height/2; y++ {
		for x := 0; x < width/2; x++ {
			img.Cb[y*img.CStride+x] = 128
			img.Cr[y*img.CStride+x] = 128
		}
	}

	data, err := encoder.Encode(img)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	// Decode using Decode method
	decoded, err := decoder.Decode(data)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if decoded == nil {
		t.Log("Frame not ready yet (buffered)")
		return
	}

	bounds := decoded.Bounds()
	if bounds.Dx() != width || bounds.Dy() != height {
		t.Errorf("unexpected frame size: got %dx%d, want %dx%d",
			bounds.Dx(), bounds.Dy(), width, height)
	}
}

func TestH264EncodeDecodeLowLatency(t *testing.T) {
	width, height := 320, 240
	reader, writer := io.Pipe()

	// Create decoder with low latency mode (ErrorConDisable)
	decParams := NewDecoderParams()
	decParams.ErrorConcealment = ErrorConDisable
	decoder, err := NewDecoderWithParams(reader, decParams)
	if err != nil {
		t.Fatalf("NewDecoderWithParams failed: %v", err)
	}
	defer decoder.Close()

	// Create encoder
	encParams := NewEncoderParams()
	encParams.Width = width
	encParams.Height = height
	encParams.MaxFrameRate = 30.0
	encParams.IntraPeriod = 1

	encoder, err := NewEncoder(encParams)
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}
	defer encoder.Close()

	totalFrames := 10
	var wg sync.WaitGroup
	wg.Add(1)

	decodedFrames := 0

	// Decoder goroutine
	go func() {
		defer wg.Done()
		for {
			img, err := decoder.Read()
			if err == io.EOF {
				return
			}
			if err != nil {
				t.Errorf("decoder read error: %v", err)
				return
			}
			bounds := img.Bounds()
			if bounds.Dx() != width || bounds.Dy() != height {
				t.Errorf("unexpected frame size: got %dx%d, want %dx%d",
					bounds.Dx(), bounds.Dy(), width, height)
			}
			decodedFrames++
		}
	}()

	// Encode and send frames
	for i := 0; i < totalFrames; i++ {
		img := image.NewYCbCr(
			image.Rect(0, 0, width, height),
			image.YCbCrSubsampleRatio420,
		)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				img.Y[y*img.YStride+x] = uint8((x + y + i*10) % 256)
			}
		}
		for y := 0; y < height/2; y++ {
			for x := 0; x < width/2; x++ {
				img.Cb[y*img.CStride+x] = 128
				img.Cr[y*img.CStride+x] = 128
			}
		}

		data, err := encoder.Encode(img)
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatalf("writer error: %v", err)
		}
	}
	writer.Close()
	wg.Wait()

	if decodedFrames != totalFrames {
		t.Errorf("frame count mismatch: got %d, want %d", decodedFrames, totalFrames)
	}
}

func TestDecoderCreateWithParams(t *testing.T) {
	r := bytes.NewReader([]byte{})

	// Test with default params
	params := NewDecoderParams()
	decoder, err := NewDecoderWithParams(r, params)
	if err != nil {
		t.Fatalf("NewDecoderWithParams failed: %v", err)
	}
	if decoder == nil {
		t.Fatal("decoder is nil")
	}
	if err := decoder.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Test with low latency params
	params.ErrorConcealment = ErrorConDisable
	decoder2, err := NewDecoderWithParams(bytes.NewReader([]byte{}), params)
	if err != nil {
		t.Fatalf("NewDecoderWithParams with low latency failed: %v", err)
	}
	if decoder2 == nil {
		t.Fatal("decoder2 is nil")
	}
	if err := decoder2.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func copyYCbCr(src *image.YCbCr) *image.YCbCr {
	bounds := src.Bounds()
	dst := image.NewYCbCr(bounds, src.SubsampleRatio)
	copy(dst.Y, src.Y)
	copy(dst.Cb, src.Cb)
	copy(dst.Cr, src.Cr)
	return dst
}

func calculatePSNR(img1, img2 *image.YCbCr) float64 {
	bounds := img1.Bounds()
	var mse float64
	count := 0

	// Calculate MSE for Y component only
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			v1 := float64(img1.Y[y*img1.YStride+x])
			v2 := float64(img2.Y[y*img2.YStride+x])
			diff := v1 - v2
			mse += diff * diff
			count++
		}
	}

	if count == 0 {
		return 0
	}
	mse /= float64(count)
	if mse == 0 {
		return math.Inf(1) // Perfect match
	}
	return 10 * math.Log10(255*255/mse)
}
