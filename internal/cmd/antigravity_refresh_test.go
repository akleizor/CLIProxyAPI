package cmd

import (
	"testing"
	"time"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestAntigravityExpiresSoon(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *cliproxyauth.Auth {
		return &cliproxyauth.Auth{Metadata: map[string]any{"expired": now.Add(d).Format(time.RFC3339)}}
	}
	if antigravityExpiresSoon(at(time.Hour), now) {
		t.Fatal("a token valid for an hour was refreshed")
	}
	if !antigravityExpiresSoon(at(5*time.Minute), now) || !antigravityExpiresSoon(at(-time.Hour), now) {
		t.Fatal("a token expiring or expired was not refreshed")
	}
	if !antigravityExpiresSoon(&cliproxyauth.Auth{Metadata: map[string]any{}}, now) {
		t.Fatal("a token with no expiry was not refreshed")
	}
}
