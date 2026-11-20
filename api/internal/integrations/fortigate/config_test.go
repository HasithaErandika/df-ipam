package fortigate

import (
	"errors"
	"testing"
)

func TestClientRefusesWhileSwitchedOff(t *testing.T) {
	cfg := Config{Active: false, URL: "https://10.0.0.1", token: "tok", VerifyTLS: true}
	if _, err := cfg.Client(); !errors.Is(err, ErrSwitchedOff) {
		t.Fatalf("a switched-off connection must not build a client, got %v", err)
	}
	cfg.Active = true
	if _, err := cfg.Client(); err != nil {
		t.Fatalf("a switched-on connection with an address and token must build a client: %v", err)
	}
}
