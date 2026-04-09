package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClientDisablesKeepAlives(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}

	transport, ok := client.HTTPClient().Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected HTTP client transport to be *http.Transport")
	}
	if !transport.DisableKeepAlives {
		t.Fatal("expected keep-alives to be disabled")
	}
}

func TestLoginReturnsDeviceErrorMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Invalid password"}`))
	}))
	defer srv.Close()

	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}

	err = client.Login(context.Background(), srv.URL, "wrong")
	if err == nil || err.Error() != "Invalid password" {
		t.Fatalf("login error = %v, want Invalid password", err)
	}
	var authErr *Error
	if !errors.As(err, &authErr) {
		t.Fatal("expected login error to unwrap to *auth.Error")
	}
	if authErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status code = %d, want %d", authErr.StatusCode, http.StatusUnauthorized)
	}
}

func TestLoginReturnsRetryAfterMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"Too many failed attempts. Please try again later.","retry_after":312}`))
	}))
	defer srv.Close()

	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}

	err = client.Login(context.Background(), srv.URL, "wrong")
	want := "Too many failed attempts. Please try again later. (retry after 312s)"
	if err == nil || err.Error() != want {
		t.Fatalf("login error = %v, want %q", err, want)
	}
	var authErr *Error
	if !errors.As(err, &authErr) {
		t.Fatal("expected login error to unwrap to *auth.Error")
	}
	if authErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status code = %d, want %d", authErr.StatusCode, http.StatusTooManyRequests)
	}
}
