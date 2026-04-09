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

func TestNormalizeBaseURLRejectsInvalidHost(t *testing.T) {
	for _, value := range []string{"bad host", "://broken", "http://bad host"} {
		if _, err := normalizeBaseURL(value); err == nil {
			t.Fatalf("expected error for %q", value)
		}
	}
}

func TestIsValidConnectHost(t *testing.T) {
	valid := []string{"192.168.1.50", "jetkvm.local", "jetkvm-22fef15037dbb5bb.isobits.local"}
	for _, value := range valid {
		if !isValidConnectHost(value) {
			t.Fatalf("expected %q to be valid", value)
		}
	}

	invalid := []string{"", "bad host", "-jetkvm.local", "jetkvm_.local", "foo/bar"}
	for _, value := range invalid {
		if isValidConnectHost(value) {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}
