#pragma once

#include <openh264/codec_api.h>

#ifdef __cplusplus
extern "C" {
#endif
typedef struct Slice {
  unsigned char *data;
  int data_len;
} Slice;

typedef struct Frame {
  void *y, *u, *v;
  int ystride;
  int cstride;
  int height;
  int width;
} Frame;

typedef struct EncoderOptions {
  int width, height;
  int target_bitrate;
  float max_fps;
  EUsageType usage_type;
  RC_MODES rc_mode;
  bool enable_frame_skip;
  unsigned int max_nal_size;
  unsigned int intra_period;
  int multiple_thread_idc;
  unsigned int slice_num;
  SliceModeEnum slice_mode;
  unsigned int slice_size_constraint;
} EncoderOptions;

typedef struct Encoder {
  SEncParamExt params;
  ISVCEncoder *engine;
  unsigned char *buff;
  int buff_size;
  int force_key_frame;
} Encoder;

Encoder *enc_new(const EncoderOptions params, int *eresult);
void enc_free(Encoder *e, int *eresult);
Slice enc_encode(Encoder *e, Frame f, int *eresult);
void enc_set_bitrate(Encoder *e, int bitrate);

// Decoder structures and functions
typedef struct DecoderOptions {
  int error_concealment;  // ERROR_CON_IDC value
} DecoderOptions;

typedef struct DecodedFrame {
  unsigned char *y, *u, *v;
  int y_stride, uv_stride;
  int width, height;
  int buffer_status;  // 0: not ready, 1: frame ready
} DecodedFrame;

typedef struct Decoder {
  ISVCDecoder *engine;
  unsigned char *dst[3];  // Y, U, V plane pointers
  SBufferInfo buffer_info;
} Decoder;

Decoder *dec_new(DecoderOptions opts, int *eresult);
void dec_free(Decoder *d, int *eresult);
DecodedFrame dec_decode(Decoder *d, unsigned char *data, int data_len, int *eresult);
DecodedFrame dec_flush(Decoder *d, int *eresult);

#ifdef __cplusplus
}
#endif
