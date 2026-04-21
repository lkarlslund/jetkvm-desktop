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

// EncoderParams stores libopenh264 specific encoding parameters.
type EncoderParams struct {
	Width               int
	Height              int
	BitRate             int
	MaxFrameRate        float32
	UsageType           UsageTypeEnum
	RCMode              RCModeEnum
	EnableFrameSkip     bool
	MaxNalSize          uint
	IntraPeriod         uint
	MultipleThreadIdc   int
	SliceNum            uint
	SliceMode           SliceModeEnum
	SliceSizeConstraint uint
}

type UsageTypeEnum int

const (
	CameraVideoRealTime      UsageTypeEnum = C.CAMERA_VIDEO_REAL_TIME
	ScreenContentRealTime    UsageTypeEnum = C.SCREEN_CONTENT_REAL_TIME
	CameraVideoNonRealTime   UsageTypeEnum = C.CAMERA_VIDEO_NON_REAL_TIME
	ScreenContentNonRealTime UsageTypeEnum = C.SCREEN_CONTENT_NON_REAL_TIME
	InputContentTypeAll      UsageTypeEnum = C.INPUT_CONTENT_TYPE_ALL
)

type RCModeEnum int

const (
	RCQualityMode         RCModeEnum = C.RC_QUALITY_MODE
	RCBitrateMode         RCModeEnum = C.RC_BITRATE_MODE
	RCBufferbaseedMode    RCModeEnum = C.RC_BUFFERBASED_MODE
	RCTimestampMode       RCModeEnum = C.RC_TIMESTAMP_MODE
	RCBitrateModePostSkip RCModeEnum = C.RC_BITRATE_MODE_POST_SKIP
	RCOffMode             RCModeEnum = C.RC_OFF_MODE
)

type SliceModeEnum uint

const (
	SMSingleSlice      SliceModeEnum = C.SM_SINGLE_SLICE
	SMFixedslcnumSlice SliceModeEnum = C.SM_FIXEDSLCNUM_SLICE
	SMRasterSlice      SliceModeEnum = C.SM_RASTER_SLICE
	SMSizelimitedSlice SliceModeEnum = C.SM_SIZELIMITED_SLICE
)

// NewEncoderParams returns default openh264 encoder parameters.
func NewEncoderParams() EncoderParams {
	return EncoderParams{
		Width:               640,
		Height:              480,
		BitRate:             100000,
		MaxFrameRate:        30.0,
		UsageType:           CameraVideoRealTime,
		RCMode:              RCBitrateMode,
		EnableFrameSkip:     true,
		MaxNalSize:          0,
		IntraPeriod:         30,
		MultipleThreadIdc:   0,
		SliceNum:            1,
		SliceMode:           SMSizelimitedSlice,
		SliceSizeConstraint: 12800,
	}
}

// Encoder represents an H.264 encoder
type Encoder struct {
	engine *C.Encoder

	mu     sync.Mutex
	closed bool
}

// NewEncoder creates a new OpenH264 encoder with the given parameters
func NewEncoder(params EncoderParams) (*Encoder, error) {
	if params.BitRate == 0 {
		params.BitRate = 100000
	}

	var rv C.int
	cEncoder := C.enc_new(C.EncoderOptions{
		width:                 C.int(params.Width),
		height:                C.int(params.Height),
		target_bitrate:        C.int(params.BitRate),
		max_fps:               C.float(params.MaxFrameRate),
		usage_type:            C.EUsageType(params.UsageType),
		rc_mode:               C.RC_MODES(params.RCMode),
		enable_frame_skip:     C.bool(params.EnableFrameSkip),
		max_nal_size:          C.uint(params.MaxNalSize),
		intra_period:          C.uint(params.IntraPeriod),
		multiple_thread_idc:   C.int(params.MultipleThreadIdc),
		slice_num:             C.uint(params.SliceNum),
		slice_mode:            C.SliceModeEnum(params.SliceMode),
		slice_size_constraint: C.uint(params.SliceSizeConstraint),
	}, &rv)
	if err := errResult(rv); err != nil {
		return nil, fmt.Errorf("failed in creating encoder: %v", err)
	}

	return &Encoder{
		engine: cEncoder,
	}, nil
}

// Encode encodes a YCbCr image and returns the encoded H.264 data
func (e *Encoder) Encode(img *image.YCbCr) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil, io.EOF
	}

	bounds := img.Bounds()
	var rv C.int
	s := C.enc_encode(e.engine, C.Frame{
		y:       unsafe.Pointer(&img.Y[0]),
		u:       unsafe.Pointer(&img.Cb[0]),
		v:       unsafe.Pointer(&img.Cr[0]),
		ystride: C.int(img.YStride),
		cstride: C.int(img.CStride),
		height:  C.int(bounds.Max.Y - bounds.Min.Y),
		width:   C.int(bounds.Max.X - bounds.Min.X),
	}, &rv)
	if err := errResult(rv); err != nil {
		return nil, fmt.Errorf("failed in encoding: %v", err)
	}

	encoded := C.GoBytes(unsafe.Pointer(s.data), s.data_len)
	return encoded, nil
}

// ForceKeyFrame forces the next frame to be encoded as a key frame
func (e *Encoder) ForceKeyFrame() error {
	e.engine.force_key_frame = C.int(1)
	return nil
}

// SetBitRate sets the target bitrate dynamically
func (e *Encoder) SetBitRate(bitrate int) error {
	C.enc_set_bitrate(e.engine, C.int(bitrate))
	return nil
}

// Close releases the encoder resources
func (e *Encoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}

	e.closed = true

	var rv C.int
	C.enc_free(e.engine, &rv)
	return errResult(rv)
}
