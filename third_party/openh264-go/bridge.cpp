#include "bridge.hpp"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

Encoder *enc_new(const EncoderOptions opts, int *eresult) {
  int rv;
  ISVCEncoder *engine = NULL;
  SEncParamExt params;

  *eresult = 0;

  rv = WelsCreateSVCEncoder(&engine);
  if (rv != 0) {
    *eresult = rv;
    return NULL;
  }

  rv = engine->GetDefaultParams(&params);
  if (rv != 0) {
    *eresult = rv;
    WelsDestroySVCEncoder(engine);
    return NULL;
  }

  params.iUsageType = opts.usage_type;
  params.iPicWidth = opts.width;
  params.iPicHeight = opts.height;
  params.iTargetBitrate = opts.target_bitrate;
  params.iMaxBitrate = opts.target_bitrate;
  params.iRCMode = opts.rc_mode;
  params.fMaxFrameRate = opts.max_fps;
  params.bEnableFrameSkip = opts.enable_frame_skip;
  params.uiMaxNalSize = opts.max_nal_size;
  params.uiIntraPeriod = opts.intra_period;
  params.iMultipleThreadIdc = opts.multiple_thread_idc;
  // The base spatial layer 0 is the only one we use.
  params.sSpatialLayers[0].iVideoWidth = params.iPicWidth;
  params.sSpatialLayers[0].iVideoHeight = params.iPicHeight;
  params.sSpatialLayers[0].fFrameRate = params.fMaxFrameRate;
  params.sSpatialLayers[0].iSpatialBitrate = params.iTargetBitrate;
  params.sSpatialLayers[0].iMaxSpatialBitrate = params.iTargetBitrate;
  params.sSpatialLayers[0].sSliceArgument.uiSliceNum = opts.slice_num;
  params.sSpatialLayers[0].sSliceArgument.uiSliceMode = opts.slice_mode;
  params.sSpatialLayers[0].sSliceArgument.uiSliceSizeConstraint = opts.slice_size_constraint;

  rv = engine->InitializeExt(&params);
  if (rv != 0) {
    *eresult = rv;
    WelsDestroySVCEncoder(engine);
    return NULL;
  }

  Encoder *encoder = (Encoder *)malloc(sizeof(Encoder));
  if (encoder == NULL) {
    *eresult = -3;  // Out of memory
    engine->Uninitialize();
    WelsDestroySVCEncoder(engine);
    return NULL;
  }

  encoder->engine = engine;
  encoder->params = params;
  encoder->buff = (unsigned char *)malloc(opts.width * opts.height);
  if (encoder->buff == NULL) {
    *eresult = -3;  // Out of memory
    engine->Uninitialize();
    WelsDestroySVCEncoder(engine);
    free(encoder);
    return NULL;
  }
  encoder->buff_size = opts.width * opts.height;
  encoder->force_key_frame = 0;
  return encoder;
}

void enc_free(Encoder *e, int *eresult) {
  *eresult = 0;

  if (e == NULL) {
    return;
  }

  int rv = e->engine->Uninitialize();
  if (rv != 0) {
    *eresult = rv;
    // Continue to destroy even if Uninitialize fails
  }

  WelsDestroySVCEncoder(e->engine);

  free(e->buff);
  free(e);
}

void enc_set_bitrate(Encoder *e, int bitrate) {
  SEncParamExt encParamExt;
  e->engine->GetOption(ENCODER_OPTION_SVC_ENCODE_PARAM_EXT, &encParamExt);
  encParamExt.iTargetBitrate=bitrate;
  encParamExt.iMaxBitrate=bitrate;
  encParamExt.sSpatialLayers[0].iSpatialBitrate = bitrate;
  encParamExt.sSpatialLayers[0].iMaxSpatialBitrate = bitrate;
  e->engine->SetOption(ENCODER_OPTION_SVC_ENCODE_PARAM_EXT, &encParamExt);
}

// There's a good reference from ffmpeg in using the encode_frame
// Reference: https://ffmpeg.org/doxygen/2.6/libopenh264enc_8c_source.html
Slice enc_encode(Encoder *e, Frame f, int *eresult) {
  int rv;
  SSourcePicture pic = {0};
  SFrameBSInfo info = {0};
  Slice payload = {0};

  *eresult = 0;

  if(e->force_key_frame == 1) {
    e->engine->ForceIntraFrame(true);
    e->force_key_frame = 0;
  }

  pic.iPicWidth = f.width;
  pic.iPicHeight = f.height;
  pic.iColorFormat = videoFormatI420;
  pic.iStride[0] = f.ystride;
  pic.iStride[1] = pic.iStride[2] = f.cstride;
  pic.pData[0] = (unsigned char *)f.y;
  pic.pData[1] = (unsigned char *)f.u;
  pic.pData[2] = (unsigned char *)f.v;

  rv = e->engine->EncodeFrame(&pic, &info);
  if (rv != 0) {
    *eresult = rv;
    return payload;
  }

  int *layer_size = (int *)calloc(sizeof(int), info.iLayerNum);
  if (layer_size == NULL) {
    *eresult = -3;  // Out of memory
    return payload;
  }

  int size = 0;
  for (int layer = 0; layer < info.iLayerNum; layer++) {
    for (int i = 0; i < info.sLayerInfo[layer].iNalCount; i++)
      layer_size[layer] += info.sLayerInfo[layer].pNalLengthInByte[i];

    size += layer_size[layer];
  }

  if (e->buff_size < size) {
    unsigned char *new_buff = (unsigned char *)realloc(e->buff, size);
    if (new_buff == NULL) {
      free(layer_size);
      *eresult = -3;  // Out of memory
      return payload;
    }
    e->buff = new_buff;
    e->buff_size = size;
  }

  size = 0;
  for (int layer = 0; layer < info.iLayerNum; layer++) {
    memcpy(e->buff + size, info.sLayerInfo[layer].pBsBuf, layer_size[layer]);
    size += layer_size[layer];
  }
  free(layer_size);

  payload.data = e->buff;
  payload.data_len = size;
  return payload;
}

// Decoder implementation
Decoder *dec_new(DecoderOptions opts, int *eresult) {
  long rv;
  ISVCDecoder *engine = NULL;
  SDecodingParam params = {0};

  *eresult = 0;

  rv = WelsCreateDecoder(&engine);
  if (rv != 0) {
    *eresult = (int)rv;
    return NULL;
  }

  // Set decoding parameters
  params.sVideoProperty.eVideoBsType = VIDEO_BITSTREAM_AVC;
  params.bParseOnly = false;
  params.eEcActiveIdc = (ERROR_CON_IDC)opts.error_concealment;

  rv = engine->Initialize(&params);
  if (rv != 0) {
    *eresult = (int)rv;
    WelsDestroyDecoder(engine);
    return NULL;
  }

  Decoder *decoder = (Decoder *)malloc(sizeof(Decoder));
  if (decoder == NULL) {
    *eresult = -3;  // Out of memory
    engine->Uninitialize();
    WelsDestroyDecoder(engine);
    return NULL;
  }

  decoder->engine = engine;
  memset(decoder->dst, 0, sizeof(decoder->dst));
  memset(&decoder->buffer_info, 0, sizeof(decoder->buffer_info));

  return decoder;
}

void dec_free(Decoder *d, int *eresult) {
  *eresult = 0;

  if (d == NULL) {
    return;
  }

  long rv = d->engine->Uninitialize();
  if (rv != 0) {
    *eresult = (int)rv;
    // Continue to destroy even if Uninitialize fails
  }

  WelsDestroyDecoder(d->engine);
  free(d);
}

DecodedFrame dec_decode(Decoder *d, unsigned char *data, int data_len, int *eresult) {
  DecodedFrame frame = {0};
  DECODING_STATE state;

  *eresult = 0;

  memset(&d->buffer_info, 0, sizeof(d->buffer_info));

  // Use DecodeFrameNoDelay (recommended API)
  state = d->engine->DecodeFrameNoDelay(data, data_len, d->dst, &d->buffer_info);

  // Check for errors (dsErrorFree = 0, dsFramePending = 1 are OK)
  if (state != dsErrorFree && state != dsFramePending) {
    *eresult = (int)state;
    return frame;
  }

  // Check if frame is ready
  if (d->buffer_info.iBufferStatus == 1) {
    frame.y = d->dst[0];
    frame.u = d->dst[1];
    frame.v = d->dst[2];
    frame.y_stride = d->buffer_info.UsrData.sSystemBuffer.iStride[0];
    frame.uv_stride = d->buffer_info.UsrData.sSystemBuffer.iStride[1];
    frame.width = d->buffer_info.UsrData.sSystemBuffer.iWidth;
    frame.height = d->buffer_info.UsrData.sSystemBuffer.iHeight;
    frame.buffer_status = 1;
  }

  return frame;
}

DecodedFrame dec_flush(Decoder *d, int *eresult) {
  DecodedFrame frame = {0};
  DECODING_STATE state;

  *eresult = 0;

  memset(&d->buffer_info, 0, sizeof(d->buffer_info));

  // Flush remaining frames from decoder buffer
  state = d->engine->FlushFrame(d->dst, &d->buffer_info);

  if (state != dsErrorFree && state != dsFramePending) {
    *eresult = (int)state;
    return frame;
  }

  // Check if frame is ready
  if (d->buffer_info.iBufferStatus == 1) {
    frame.y = d->dst[0];
    frame.u = d->dst[1];
    frame.v = d->dst[2];
    frame.y_stride = d->buffer_info.UsrData.sSystemBuffer.iStride[0];
    frame.uv_stride = d->buffer_info.UsrData.sSystemBuffer.iStride[1];
    frame.width = d->buffer_info.UsrData.sSystemBuffer.iWidth;
    frame.height = d->buffer_info.UsrData.sSystemBuffer.iHeight;
    frame.buffer_status = 1;
  }

  return frame;
}
