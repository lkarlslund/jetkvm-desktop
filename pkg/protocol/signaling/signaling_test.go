package signaling

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/websocket"
)

func TestDialWebsocketSendsDeviceOriginAndSessionCookie(t *testing.T) {
	var gotOrigin, gotCookie, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrigin = r.Header.Get("Origin")
		gotCookie = r.Header.Get("Cookie")
		gotPath = r.URL.Path
		if gotOrigin != "http://"+r.Host {
			http.Error(w, `{"error":"Device UI origin required"}`, http.StatusForbidden)
			return
		}
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
			return r.Header.Get("Origin") == "http://"+r.Host
		}}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer srv.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	deviceURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(deviceURL, []*http.Cookie{{Name: "session", Value: "example"}})
	client := &http.Client{Jar: jar}

	conn, response, err := DialWebsocket(context.Background(), client, srv.URL)
	if err != nil {
		t.Fatalf("DialWebsocket: %v (response: %v)", err, response)
	}
	defer conn.Close()

	if gotOrigin != srv.URL {
		t.Errorf("Origin = %q, want %q", gotOrigin, srv.URL)
	}
	if gotCookie != "session=example" {
		t.Errorf("Cookie = %q, want session=example", gotCookie)
	}
	if gotPath != "/webrtc/signaling/client" {
		t.Errorf("path = %q, want /webrtc/signaling/client", gotPath)
	}
}

func TestDialWebsocketOriginDoesNotIncludePath(t *testing.T) {
	var gotOrigin string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrigin = r.Header.Get("Origin")
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer srv.Close()

	conn, resp, err := DialWebsocket(context.Background(), nil, srv.URL+"/some/path/")
	if err != nil {
		t.Fatalf("DialWebsocket: %v (response: %v)", err, resp)
	}
	defer conn.Close()
	if gotOrigin != srv.URL {
		t.Errorf("Origin = %q, want %q", gotOrigin, srv.URL)
	}
}
