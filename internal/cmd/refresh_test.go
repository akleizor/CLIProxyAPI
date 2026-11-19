package cmd

import (
	"testing"
	"time"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestExpiresSoon(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *cliproxyauth.Auth {
		return &cliproxyauth.Auth{Metadata: map[string]any{"expired": now.Add(d).Format(time.RFC3339)}}
	}
	if expiresSoon(at(time.Hour), now) {
		t.Fatal("a token valid for an hour was refreshed")
	}
	if !expiresSoon(at(5*time.Minute), now) || !expiresSoon(at(-time.Hour), now) {
		t.Fatal("a token expiring or expired was not refreshed")
	}
	if expiresSoon(&cliproxyauth.Auth{Metadata: map[string]any{}}, now) {
		t.Fatal("a token with no expiry was refreshed")
	}
}

func TestRefreshExecutorCoversOAuthProviders(t *testing.T) {
	for _, p := range []string{"antigravity", "codex", "claude", "gemini", "kimi", "xai", "devin", "meta"} {
		if refreshExecutor(nil, p) == nil {
			t.Fatalf("%s has no refresher", p)
		}
	}
	if refreshExecutor(nil, "openai-compatibility") != nil {
		t.Fatal("an API-key provider was given a refresher")
	}
}
