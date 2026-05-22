//go:build ffmpeg && cgo

package video

/*
#cgo pkg-config: libavcodec libavutil libswscale
#include <libavcodec/avcodec.h>
#include <libavutil/avutil.h>
#include <libavutil/hwcontext.h>
#include <libavutil/imgutils.h>
#include <libswscale/swscale.h>
#include <stdlib.h>
#include <string.h>

static enum AVPixelFormat hw_pix_fmt = AV_PIX_FMT_NONE;

static enum AVPixelFormat get_hw_format(AVCodecContext *ctx,
                                        const enum AVPixelFormat *pix_fmts) {
    for (const enum AVPixelFormat *p = pix_fmts; *p != AV_PIX_FMT_NONE; p++) {
        if (*p == hw_pix_fmt)
            return *p;
    }
    return pix_fmts[0];
}

#define NUM_RGBA_BUFFERS 3

typedef struct {
    AVCodecContext   *codec_ctx;
    AVBufferRef     *hw_device_ctx;
    AVFrame         *frame;
    AVFrame         *sw_frame;
    AVPacket        *pkt;
    struct SwsContext *sws;
    int              sws_w;
    int              sws_h;
    enum AVPixelFormat sws_fmt;
    int              is_hw;

    // Ring of RGBA output buffers. swscale writes directly into one of them
    // each frame. The decoder returns a pointer + length; the caller can
    // safely read the buffer until the next 2 Decode() calls.
    uint8_t         *rgba[NUM_RGBA_BUFFERS];
    int              buf_w;
    int              buf_h;
    int              write_idx;
} FFDecoder;

static FFDecoder *ff_decoder_open(int *out_hw) {
    *out_hw = 0;
    av_log_set_level(AV_LOG_FATAL);

    const AVCodec *codec = avcodec_find_decoder(AV_CODEC_ID_H264);
    if (!codec) return NULL;

    FFDecoder *d = calloc(1, sizeof(FFDecoder));
    if (!d) return NULL;

    d->codec_ctx = avcodec_alloc_context3(codec);
    if (!d->codec_ctx) { free(d); return NULL; }

    enum AVHWDeviceType hw_type = AV_HWDEVICE_TYPE_VAAPI;
    for (int i = 0;; i++) {
        const AVCodecHWConfig *config = avcodec_get_hw_config(codec, i);
        if (!config) break;
        if (config->methods & AV_CODEC_HW_CONFIG_METHOD_HW_DEVICE_CTX &&
            config->device_type == hw_type) {
            hw_pix_fmt = config->pix_fmt;
            break;
        }
    }

    if (hw_pix_fmt != AV_PIX_FMT_NONE) {
        if (av_hwdevice_ctx_create(&d->hw_device_ctx, hw_type, NULL, NULL, 0) >= 0) {
            d->codec_ctx->hw_device_ctx = av_buffer_ref(d->hw_device_ctx);
            d->codec_ctx->get_format = get_hw_format;
            d->is_hw = 1;
            *out_hw = 1;
        } else {
            hw_pix_fmt = AV_PIX_FMT_NONE;
        }
    }

    d->codec_ctx->flags  |= AV_CODEC_FLAG_LOW_DELAY;
    d->codec_ctx->flags2 |= AV_CODEC_FLAG2_FAST;
    // VA-API decode happens on GPU so 1 thread is enough; software fallback
    // benefits from multi-threading but we cap at 2 to leave a core for the
    // renderer on dual-core hardware.
    d->codec_ctx->thread_count = 2;
    d->codec_ctx->err_recognition = 0;

    if (avcodec_open2(d->codec_ctx, codec, NULL) < 0) {
        avcodec_free_context(&d->codec_ctx);
        if (d->hw_device_ctx) av_buffer_unref(&d->hw_device_ctx);
        free(d);
        return NULL;
    }

    d->frame    = av_frame_alloc();
    d->sw_frame = av_frame_alloc();
    d->pkt      = av_packet_alloc();
    if (!d->frame || !d->sw_frame || !d->pkt) {
        avcodec_free_context(&d->codec_ctx);
        if (d->hw_device_ctx) av_buffer_unref(&d->hw_device_ctx);
        av_frame_free(&d->frame);
        av_frame_free(&d->sw_frame);
        av_packet_free(&d->pkt);
        free(d);
        return NULL;
    }
    return d;
}

static void ff_decoder_close(FFDecoder *d) {
    if (!d) return;
    if (d->sws) sws_freeContext(d->sws);
    av_frame_free(&d->frame);
    av_frame_free(&d->sw_frame);
    av_packet_free(&d->pkt);
    avcodec_free_context(&d->codec_ctx);
    if (d->hw_device_ctx) av_buffer_unref(&d->hw_device_ctx);
    for (int i = 0; i < NUM_RGBA_BUFFERS; i++) free(d->rgba[i]);
    free(d);
}

// Returns: 0 = frame ready, 1 = need more data, <0 = error.
// out_ptr is set to a pointer into one of our internal RGBA buffers — caller
// must NOT free it. Buffer remains valid until 2 more successful calls.
//
// IMPORTANT: drains all frames the codec produces from this packet, keeping
// only the latest. WebRTC H.264 may emit multiple frames per send_packet
// when there are SPS/PPS units or B-frame reordering, and we want the most
// recent — never replay stale frames.
static int ff_decode(FFDecoder *d, const uint8_t *data, int size,
                     int *out_w, int *out_h, uint8_t **out_ptr) {
    d->pkt->data = (uint8_t *)data;
    d->pkt->size = size;

    int ret = avcodec_send_packet(d->codec_ctx, d->pkt);
    if (ret < 0 && ret != AVERROR(EAGAIN))
        return 1;

    // Drain every queued frame; the loop exits with d->frame holding the
    // latest one (or no frame at all, in which case have_frame stays 0).
    int have_frame = 0;
    while (1) {
        AVFrame *tmp = av_frame_alloc();
        if (!tmp) break;
        ret = avcodec_receive_frame(d->codec_ctx, tmp);
        if (ret == AVERROR(EAGAIN) || ret == AVERROR_EOF) {
            av_frame_free(&tmp);
            break;
        }
        if (ret < 0) {
            av_frame_free(&tmp);
            break;
        }
        // Got one — swap it in as the current frame, free any prior.
        av_frame_unref(d->frame);
        av_frame_move_ref(d->frame, tmp);
        av_frame_free(&tmp);
        have_frame = 1;
    }
    if (!have_frame) {
        return 1;
    }

    AVFrame *src = d->frame;

    // d->is_hw means we successfully opened a VA-API device but each frame
    // might still come back as software (e.g. when the bitstream uses a
    // profile the hardware can't handle). Track this so the caller can
    // see what's actually being used.
    static int last_was_hw = -1;
    int frame_is_hw = (d->is_hw && src->format == hw_pix_fmt) ? 1 : 0;
    if (frame_is_hw != last_was_hw) {
        fprintf(stderr, "[ffmpeg] frame format=%d, hw_pix_fmt=%d, using %s\n",
                src->format, hw_pix_fmt, frame_is_hw ? "GPU" : "CPU");
        last_was_hw = frame_is_hw;
    }

    if (frame_is_hw) {
        av_frame_unref(d->sw_frame);
        ret = av_hwframe_transfer_data(d->sw_frame, src, 0);
        if (ret < 0)
            return 1;
        src = d->sw_frame;
    }

    int w = src->width;
    int h = src->height;
    if (w <= 0 || h <= 0)
        return 1;

    // Allocate/resize ring buffers on resolution change.
    if (d->buf_w != w || d->buf_h != h) {
        for (int i = 0; i < NUM_RGBA_BUFFERS; i++) {
            free(d->rgba[i]);
            d->rgba[i] = malloc(w * h * 4);
            if (!d->rgba[i]) return -1;
        }
        d->buf_w = w;
        d->buf_h = h;
        d->write_idx = 0;
    }

    // Rotate to the next buffer in the ring.
    d->write_idx = (d->write_idx + 1) % NUM_RGBA_BUFFERS;
    uint8_t *dst = d->rgba[d->write_idx];

    // GPU-side YCbCr→RGB conversion is much cheaper than CPU swscale.
    // We pack each pixel as 4 bytes: Y, Cb (upsampled), Cr (upsampled), 0xff.
    // The shader then samples this buffer and applies the colour matrix.
    //
    // Fast paths for the two formats VA-API/libav emit on Linux:
    //   - NV12 (VA-API hw transfer): Y plane + interleaved CbCr at 1/2 res.
    //   - YUV420P (software decode): separate Y, Cb, Cr planes at 1/2 res.
    int dst_stride = w * 4;
    if (src->format == AV_PIX_FMT_NV12) {
        const uint8_t *y_plane  = src->data[0];
        const uint8_t *uv_plane = src->data[1];
        int y_stride  = src->linesize[0];
        int uv_stride = src->linesize[1];
        for (int j = 0; j < h; j++) {
            const uint8_t *y_row  = y_plane  + j * y_stride;
            const uint8_t *uv_row = uv_plane + (j / 2) * uv_stride;
            uint8_t *out_row = dst + j * dst_stride;
            for (int i = 0; i < w; i++) {
                int uv_i = (i / 2) * 2;
                out_row[i * 4 + 0] = y_row[i];
                out_row[i * 4 + 1] = uv_row[uv_i];     // Cb
                out_row[i * 4 + 2] = uv_row[uv_i + 1]; // Cr
                out_row[i * 4 + 3] = 0xff;
            }
        }
    } else if (src->format == AV_PIX_FMT_YUV420P || src->format == AV_PIX_FMT_YUVJ420P) {
        const uint8_t *y_plane  = src->data[0];
        const uint8_t *cb_plane = src->data[1];
        const uint8_t *cr_plane = src->data[2];
        int y_stride  = src->linesize[0];
        int cb_stride = src->linesize[1];
        int cr_stride = src->linesize[2];
        for (int j = 0; j < h; j++) {
            const uint8_t *y_row  = y_plane  + j * y_stride;
            const uint8_t *cb_row = cb_plane + (j / 2) * cb_stride;
            const uint8_t *cr_row = cr_plane + (j / 2) * cr_stride;
            uint8_t *out_row = dst + j * dst_stride;
            for (int i = 0; i < w; i++) {
                int c_i = i / 2;
                out_row[i * 4 + 0] = y_row[i];
                out_row[i * 4 + 1] = cb_row[c_i];
                out_row[i * 4 + 2] = cr_row[c_i];
                out_row[i * 4 + 3] = 0xff;
            }
        }
    } else {
        // Exotic format — fall back to swscale to convert to YUV420P first.
        if (!d->sws || d->sws_w != w || d->sws_h != h || d->sws_fmt != src->format) {
            if (d->sws) sws_freeContext(d->sws);
            d->sws = sws_getContext(
                w, h, src->format,
                w, h, AV_PIX_FMT_YUV420P,
                SWS_POINT, NULL, NULL, NULL);
            if (!d->sws) return -1;
            d->sws_w = w;
            d->sws_h = h;
            d->sws_fmt = src->format;
        }
        uint8_t *tmp_y  = malloc(w * h);
        uint8_t *tmp_cb = malloc((w/2) * (h/2));
        uint8_t *tmp_cr = malloc((w/2) * (h/2));
        if (!tmp_y || !tmp_cb || !tmp_cr) {
            free(tmp_y); free(tmp_cb); free(tmp_cr);
            return -1;
        }
        uint8_t *tmp_data[3] = { tmp_y, tmp_cb, tmp_cr };
        int tmp_linesize[3] = { w, w/2, w/2 };
        sws_scale(d->sws,
            (const uint8_t *const *)src->data, src->linesize, 0, h,
            tmp_data, tmp_linesize);
        for (int j = 0; j < h; j++) {
            for (int i = 0; i < w; i++) {
                int c_i = i / 2;
                int c_j = j / 2;
                dst[j * dst_stride + i * 4 + 0] = tmp_y [j * w + i];
                dst[j * dst_stride + i * 4 + 1] = tmp_cb[c_j * (w/2) + c_i];
                dst[j * dst_stride + i * 4 + 2] = tmp_cr[c_j * (w/2) + c_i];
                dst[j * dst_stride + i * 4 + 3] = 0xff;
            }
        }
        free(tmp_y); free(tmp_cb); free(tmp_cr);
    }

    *out_w = w;
    *out_h = h;
    *out_ptr = dst;
    return 0;
}
*/
import "C"

import (
	"errors"
	"image"
	"unsafe"
)

type ffmpegDecoder struct {
	d    *C.FFDecoder
	isHW bool
}

func newFFmpegDecoder() (Decoder, error) {
	var hw C.int
	d := C.ff_decoder_open(&hw)
	if d == nil {
		return nil, errors.New("ffmpeg: failed to open H.264 decoder")
	}
	return &ffmpegDecoder{d: d, isHW: hw != 0}, nil
}

func (f *ffmpegDecoder) Decode(nalu []byte) (image.Image, error) {
	if len(nalu) == 0 {
		return nil, nil
	}
	var w, h C.int
	var ptr *C.uint8_t
	ret := C.ff_decode(f.d, (*C.uint8_t)(unsafe.Pointer(&nalu[0])), C.int(len(nalu)), &w, &h, &ptr)
	if ret != 0 || ptr == nil {
		return nil, nil
	}
	gw, gh := int(w), int(h)
	stride := gw * 4
	// Zero-copy: the underlying RGBA.Pix points directly into the C ring
	// buffer. Pixels are packed Y, Cb, Cr, 0xff; the app uses a GPU shader
	// to convert YCbCr→RGB at draw time. Caller (renderer) must consume
	// before 2 more Decode() calls.
	pix := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), stride*gh)
	return &PackedYCbCr{RGBA: &image.RGBA{
		Pix:    pix,
		Stride: stride,
		Rect:   image.Rect(0, 0, gw, gh),
	}}, nil
}

func (f *ffmpegDecoder) Close() error {
	if f.d != nil {
		C.ff_decoder_close(f.d)
		f.d = nil
	}
	return nil
}

func (f *ffmpegDecoder) Name() string {
	if f.isHW {
		return "ffmpeg/vaapi"
	}
	return "ffmpeg/sw"
}
