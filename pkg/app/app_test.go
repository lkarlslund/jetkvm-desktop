package app

import "testing"

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "192.168.1.50", want: "http://192.168.1.50"},
		{in: "jetkvm.local", want: "http://jetkvm.local"},
		{in: "https://jetkvm.local/view", want: "https://jetkvm.local"},
	}

	for _, tc := range tests {
		got, err := normalizeBaseURL(tc.in)
		if err != nil {
			t.Fatalf("normalizeBaseURL(%q) returned error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("normalizeBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeBaseURLRejectsEmpty(t *testing.T) {
	if _, err := normalizeBaseURL(""); err == nil {
		t.Fatal("expected error for empty input")
	}
}
