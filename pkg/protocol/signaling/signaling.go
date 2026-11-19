package signaling

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

type ExchangeRequest struct {
	SD string `json:"sd"`
}

type ExchangeResponse struct {
	SD string `json:"sd"`
}

type WSMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type DeviceMetadata struct {
	DeviceVersion string `json:"deviceVersion"`
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

func WebsocketURL(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return "", err
	}

	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported base URL scheme %q", parsed.Scheme)
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/webrtc/signaling/client"
	return parsed.String(), nil
}

func DialWebsocket(ctx context.Context, client *http.Client, baseURL string) (*websocket.Conn, *http.Response, error) {
	wsURL, err := WebsocketURL(baseURL)
	if err != nil {
		return nil, nil, err
	}

	httpURL, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return nil, nil, err
	}

	header := http.Header{}
	// The device checks the browser-style Origin header before upgrading the
	// signaling connection. A native client must supply it explicitly.
	header.Set("Origin", httpURL.Scheme+"://"+httpURL.Host)
	if client != nil && client.Jar != nil {
		for _, cookie := range client.Jar.Cookies(httpURL) {
			header.Add("Cookie", cookie.String())
		}
	}

	var dialer websocket.Dialer
	return dialer.DialContext(ctx, wsURL, header)
}
