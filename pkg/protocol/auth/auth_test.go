package auth

import (
	"net/http"
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
