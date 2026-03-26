package jsonrpc

import (
	"testing"
)

func TestDecodeMessage(t *testing.T) {
	t.Run("request", func(t *testing.T) {
		msg, err := DecodeMessage([]byte(`{"jsonrpc":"2.0","method":"ping","params":{},"id":"1"}`))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := msg.(Request); !ok {
			t.Fatalf("expected Request, got %T", msg)
		}
	})

	t.Run("response", func(t *testing.T) {
		msg, err := DecodeMessage([]byte(`{"jsonrpc":"2.0","result":"pong","id":"1"}`))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := msg.(Response); !ok {
			t.Fatalf("expected Response, got %T", msg)
		}
	})

	t.Run("event", func(t *testing.T) {
		msg, err := DecodeMessage([]byte(`{"jsonrpc":"2.0","method":"videoInputState","params":{"state":"ok"}}`))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := msg.(Event); !ok {
			t.Fatalf("expected Event, got %T", msg)
		}
	})
}
