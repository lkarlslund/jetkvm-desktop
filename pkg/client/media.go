package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lkarlslund/jetkvm-desktop/pkg/virtualmedia"
)

func (c *Client) UnmountMedia(ctx context.Context) error {
	return c.Call(ctx, "unmountImage", nil, nil)
}

func (c *Client) MountMediaURL(ctx context.Context, url string, mode virtualmedia.Mode) error {
	return c.Call(ctx, "mountWithHTTP", MountWithHTTPRequest{
		URL:  url,
		Mode: mode,
	}, nil)
}

func (c *Client) GetVirtualMediaState(ctx context.Context) (*virtualmedia.State, error) {
	var state *virtualmedia.State
	if err := c.Call(ctx, "getVirtualMediaState", nil, &state); err != nil {
		return nil, err
	}
	return state, nil
}

func (c *Client) GetStorageSpace(ctx context.Context) (virtualmedia.StorageSpace, error) {
	var space virtualmedia.StorageSpace
	err := c.Call(ctx, "getStorageSpace", nil, &space)
	return space, err
}

func (c *Client) ListStorageFiles(ctx context.Context) ([]virtualmedia.StorageFile, error) {
	var payload StorageFilesResponse
	if err := c.Call(ctx, "listStorageFiles", nil, &payload); err != nil {
		return nil, err
	}
	return payload.Files, nil
}

func (c *Client) DeleteStorageFile(ctx context.Context, filename string) error {
	return c.Call(ctx, "deleteStorageFile", DeleteStorageFileRequest{Filename: filename}, nil)
}

func (c *Client) MountStorageFile(ctx context.Context, filename string, mode virtualmedia.Mode) error {
	return c.Call(ctx, "mountWithStorage", MountWithStorageRequest{
		Filename: filename,
		Mode:     mode,
	}, nil)
}

func (c *Client) StartStorageFileUpload(ctx context.Context, filename string, size int64) (virtualmedia.UploadStart, error) {
	var start virtualmedia.UploadStart
	err := c.Call(ctx, "startStorageFileUpload", StorageUploadRequest{
		Filename: filename,
		Size:     size,
	}, &start)
	return start, err
}

func (c *Client) UploadStorageFile(ctx context.Context, uploadID string, body io.Reader, offset, total int64, progress func(virtualmedia.UploadProgress)) error {
	uploadURL, err := url.Parse(strings.TrimRight(c.cfg.BaseURL, "/") + "/storage/upload")
	if err != nil {
		return err
	}
	query := uploadURL.Query()
	query.Set("uploadId", uploadID)
	uploadURL.RawQuery = query.Encode()

	var reader io.Reader = body
	if progress != nil {
		reader = &countingReader{
			reader:   body,
			offset:   offset,
			total:    total,
			progress: progress,
			lastAt:   time.Now(),
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL.String(), reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.authClient.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if len(data) == 0 {
			return fmt.Errorf("upload failed with status %s", resp.Status)
		}
		return fmt.Errorf("upload failed: %s", strings.TrimSpace(string(data)))
	}
	if progress != nil {
		progress(virtualmedia.UploadProgress{Sent: total, Total: total})
	}
	return nil
}

type countingReader struct {
	reader      io.Reader
	offset      int64
	total       int64
	sent        int64
	progress    func(virtualmedia.UploadProgress)
	lastAt      time.Time
	lastSent    int64
	speedWindow []float64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.sent += int64(n)
		now := time.Now()
		if now.Sub(r.lastAt) >= 150*time.Millisecond || err == io.EOF {
			deltaBytes := r.sent - r.lastSent
			deltaTime := now.Sub(r.lastAt).Seconds()
			if deltaBytes > 0 && deltaTime > 0 {
				r.speedWindow = append(r.speedWindow, float64(deltaBytes)/deltaTime)
				if len(r.speedWindow) > 5 {
					r.speedWindow = r.speedWindow[len(r.speedWindow)-5:]
				}
			}
			var bytesPerS float64
			for _, sample := range r.speedWindow {
				bytesPerS += sample
			}
			if len(r.speedWindow) > 0 {
				bytesPerS /= float64(len(r.speedWindow))
			}
			r.progress(virtualmedia.UploadProgress{
				Sent:      r.offset + r.sent,
				Total:     r.total,
				BytesPerS: bytesPerS,
			})
			r.lastAt = now
			r.lastSent = r.sent
		}
	}
	return n, err
}
