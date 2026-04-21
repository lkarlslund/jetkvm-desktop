package openh264

// #include <string.h>
// #include <openh264/codec_api.h>
// #include <errno.h>
// #include "bridge.hpp"
import "C"

import (
	"fmt"
	"image"
	"io"
	"sync"
	"unsafe"
)

// ErrorConcealmentMode represents the error concealment mode for the decoder
type ErrorConcealmentMode int

const (
	ErrorConDisable   ErrorConcealmentMode = C.ERROR_CON_DISABLE
	ErrorConSliceCopy ErrorConcealmentMode = C.ERROR_CON_SLICE_COPY
)

// DecoderParams stores libopenh264 specific decoding parameters.
type DecoderParams struct {
	ErrorConcealment ErrorConcealmentMode
}

// NewDecoderParams returns default openh264 decoder parameters.
func NewDecoderParams() DecoderParams {
	return DecoderParams{
		ErrorConcealment: ErrorConSliceCopy,
	}
}

// Decoder represents an H.264 decoder
type Decoder struct {
	engine *C.Decoder
	reader io.Reader
	buf    []byte

	mu       sync.Mutex
	closed   bool
	flushing bool
}

// NewDecoder creates a new OpenH264 decoder that reads from the given reader
func NewDecoder(r io.Reader) (*Decoder, error) {
	return NewDecoderWithParams(r, NewDecoderParams())
}

// NewDecoderWithParams creates a new OpenH264 decoder with the given parameters
func NewDecoderWithParams(r io.Reader, params DecoderParams) (*Decoder, error) {
	var rv C.int

	cDecoder := C.dec_new(C.DecoderOptions{
		error_concealment: C.int(params.ErrorConcealment),
	}, &rv)
	if err := errResult(rv); err != nil {
		return nil, fmt.Errorf("failed in creating decoder: %v", err)
	}

	return &Decoder{
		engine: cDecoder,
		reader: r,
		buf:    make([]byte, 1024*1024), // 1MB initial buffer
	}, nil
}

// Read reads and decodes the next frame from the stream
func (d *Decoder) Read() (*image.YCbCr, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil, io.EOF
	}

	for {
		var frame C.DecodedFrame
		var rv C.int

		if d.flushing {
			frame = C.dec_flush(d.engine, &rv)
			if err := errResult(rv); err != nil {
				return nil, fmt.Errorf("flush failed: %v", err)
			}
			if frame.buffer_status == 0 {
				return nil, io.EOF
			}
		} else {
			n, err := d.reader.Read(d.buf)
			if err == io.EOF {
				d.flushing = true
				continue
			}
			if err != nil {
				return nil, err
			}

			frame = C.dec_decode(
				d.engine,
				(*C.uchar)(unsafe.Pointer(&d.buf[0])),
				C.int(n),
				&rv,
			)

			if err := errResult(rv); err != nil {
				return nil, fmt.Errorf("decode failed: %v", err)
			}

			if frame.buffer_status == 0 {
				continue
			}
		}

		w := int(frame.width)
		h := int(frame.height)
		yStride := int(frame.y_stride)
		uvStride := int(frame.uv_stride)

		ySrc := unsafe.Slice((*byte)(unsafe.Pointer(frame.y)), yStride*h)
		uSrc := unsafe.Slice((*byte)(unsafe.Pointer(frame.u)), uvStride*h/2)
		vSrc := unsafe.Slice((*byte)(unsafe.Pointer(frame.v)), uvStride*h/2)

		dst := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)

		for r := 0; r < h; r++ {
			copy(dst.Y[r*dst.YStride:r*dst.YStride+w], ySrc[r*yStride:r*yStride+w])
		}

		for r := 0; r < h/2; r++ {
			copy(dst.Cb[r*dst.CStride:r*dst.CStride+w/2], uSrc[r*uvStride:r*uvStride+w/2])
			copy(dst.Cr[r*dst.CStride:r*dst.CStride+w/2], vSrc[r*uvStride:r*uvStride+w/2])
		}

		return dst, nil
	}
}

// Decode decodes raw H.264 data and returns the decoded frame if available
func (d *Decoder) Decode(data []byte) (*image.YCbCr, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil, io.EOF
	}

	var rv C.int
	frame := C.dec_decode(
		d.engine,
		(*C.uchar)(unsafe.Pointer(&data[0])),
		C.int(len(data)),
		&rv,
	)

	if err := errResult(rv); err != nil {
		return nil, fmt.Errorf("decode failed: %v", err)
	}

	if frame.buffer_status == 0 {
		return nil, nil
	}

	w := int(frame.width)
	h := int(frame.height)
	yStride := int(frame.y_stride)
	uvStride := int(frame.uv_stride)

	ySrc := unsafe.Slice((*byte)(unsafe.Pointer(frame.y)), yStride*h)
	uSrc := unsafe.Slice((*byte)(unsafe.Pointer(frame.u)), uvStride*h/2)
	vSrc := unsafe.Slice((*byte)(unsafe.Pointer(frame.v)), uvStride*h/2)

	dst := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)

	for r := 0; r < h; r++ {
		copy(dst.Y[r*dst.YStride:r*dst.YStride+w], ySrc[r*yStride:r*yStride+w])
	}

	for r := 0; r < h/2; r++ {
		copy(dst.Cb[r*dst.CStride:r*dst.CStride+w/2], uSrc[r*uvStride:r*uvStride+w/2])
		copy(dst.Cr[r*dst.CStride:r*dst.CStride+w/2], vSrc[r*uvStride:r*uvStride+w/2])
	}

	return dst, nil
}

// Flush flushes any remaining frames from the decoder buffer
func (d *Decoder) Flush() (*image.YCbCr, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil, io.EOF
	}

	var rv C.int
	frame := C.dec_flush(d.engine, &rv)

	if err := errResult(rv); err != nil {
		return nil, fmt.Errorf("flush failed: %v", err)
	}

	if frame.buffer_status == 0 {
		return nil, nil
	}

	w := int(frame.width)
	h := int(frame.height)
	yStride := int(frame.y_stride)
	uvStride := int(frame.uv_stride)

	ySrc := unsafe.Slice((*byte)(unsafe.Pointer(frame.y)), yStride*h)
	uSrc := unsafe.Slice((*byte)(unsafe.Pointer(frame.u)), uvStride*h/2)
	vSrc := unsafe.Slice((*byte)(unsafe.Pointer(frame.v)), uvStride*h/2)

	dst := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)

	for r := 0; r < h; r++ {
		copy(dst.Y[r*dst.YStride:r*dst.YStride+w], ySrc[r*yStride:r*yStride+w])
	}

	for r := 0; r < h/2; r++ {
		copy(dst.Cb[r*dst.CStride:r*dst.CStride+w/2], uSrc[r*uvStride:r*uvStride+w/2])
		copy(dst.Cr[r*dst.CStride:r*dst.CStride+w/2], vSrc[r*uvStride:r*uvStride+w/2])
	}

	return dst, nil
}

// Close releases the decoder resources
func (d *Decoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}

	d.closed = true

	var rv C.int
	C.dec_free(d.engine, &rv)
	return errResult(rv)
}
