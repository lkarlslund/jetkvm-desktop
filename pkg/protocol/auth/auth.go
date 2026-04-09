package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

type Client struct {
	httpClient *http.Client
}

type Error struct {
	StatusCode int
	Message    string
	RetryAfter int
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return fmt.Sprintf("login failed with status %d", e.StatusCode)
	}
	if e.RetryAfter > 0 {
		return fmt.Sprintf("%s (retry after %ds)", e.Message, e.RetryAfter)
	}
	return e.Message
}

func NewClient() (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Client{
		httpClient: &http.Client{
			Jar: jar,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				ForceAttemptHTTP2:     false,
				MaxIdleConns:          0,
				MaxIdleConnsPerHost:   -1,
				IdleConnTimeout:       0,
				TLSHandshakeTimeout:   5 * time.Second,
				ExpectContinueTimeout: time.Second,
				DisableKeepAlives:     true,
			},
		},
	}, nil
}

func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}

func (c *Client) Login(ctx context.Context, baseURL, password string) error {
	if password == "" {
		return nil
	}

	body, err := json.Marshal(map[string]string{"password": password})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/auth/login-local", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return loginError(resp)
	}
	return nil
}

func loginError(resp *http.Response) error {
	var payload struct {
		Error      string `json:"error"`
		RetryAfter int    `json:"retry_after"`
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if len(data) > 0 && json.Unmarshal(data, &payload) == nil && payload.Error != "" {
		return &Error{
			StatusCode: resp.StatusCode,
			Message:    payload.Error,
			RetryAfter: payload.RetryAfter,
		}
	}
	return &Error{
		StatusCode: resp.StatusCode,
		Message:    fmt.Sprintf("login failed with status %s", resp.Status),
	}
}
