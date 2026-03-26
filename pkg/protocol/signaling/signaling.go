package signaling

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type ExchangeRequest struct {
	SD string `json:"sd"`
}

type ExchangeResponse struct {
	SD string `json:"sd"`
}

func EncodeSDP(raw []byte) string {
	return base64.StdEncoding.EncodeToString(raw)
}

func DecodeSDP(encoded string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(encoded)
}

func Exchange(ctx context.Context, client *http.Client, baseURL string, req ExchangeRequest) (ExchangeResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return ExchangeResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/webrtc/session", bytes.NewReader(body))
	if err != nil {
		return ExchangeResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return ExchangeResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ExchangeResponse{}, fmt.Errorf("signaling failed with status %s", resp.Status)
	}

	var out ExchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ExchangeResponse{}, err
	}
	return out, nil
}
