package cmd

import "testing"

func TestProfileDisplayValues(t *testing.T) {
	if got := displayConfigValue("alice"); got != "alice" {
		t.Fatalf("display value = %q, want alice", got)
	}
	if got := displayConfigValue(""); got != "(not set)" {
		t.Fatalf("empty display value = %q, want (not set)", got)
	}
	if got := maskSensitiveValue("secret"); got != "******" {
		t.Fatalf("masked value = %q, want ******", got)
	}
	if got := maskSensitiveValue(""); got != "(not set)" {
		t.Fatalf("empty masked value = %q, want (not set)", got)
	}
}
