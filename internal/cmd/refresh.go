package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/runtime/executor"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
	sdkAuth "github.com/router-for-me/CLIProxyAPI/v8/sdk/auth"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

// refreshLead refreshes a sign-in this close to expiry, so a caller that reads
// the token right after gets one that outlives its request.
const refreshLead = 10 * time.Minute

// refreshExecutor is the provider's own executor, whose Refresh holds its OAuth
// client; nil for a provider with nothing to refresh (API keys).
func refreshExecutor(cfg *config.Config, provider string) cliproxyauth.ProviderExecutor {
	switch strings.ToLower(provider) {
	case "antigravity":
		return executor.NewAntigravityExecutor(cfg)
	case "codex":
		return executor.NewCodexExecutor(cfg)
	case "claude":
		return executor.NewClaudeExecutor(cfg)
	case "gemini", "gemini-cli":
		return executor.NewGeminiExecutor(cfg)
	case "kimi", "kimi-ai", "kimi.ai", "kimi.com":
		return executor.NewKimiExecutor(cfg)
	case "xai":
		return executor.NewXAIExecutor(cfg)
	case "devin":
		return executor.NewDevinExecutor(cfg)
	case "meta":
		return executor.NewMetaExecutor(cfg)
	}
	return nil
}

// DoRefresh refreshes every OAuth sign-in in the auth dir that expires soon,
// through its provider's own executor, saves it and exits. It lets a tool that
// only reads tokens (ccmorph's quota reader) keep sign-ins alive with no proxy
// running.
func DoRefresh(cfg *config.Config) int {
	dir, err := util.ResolveAuthDir(cfg.AuthDir)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	store := sdkAuth.NewFileTokenStore()
	store.SetBaseDir(dir)
	ctx := context.Background()
	auths, err := store.List(ctx)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	code := 0
	for _, a := range auths {
		exec := refreshExecutor(cfg, a.Provider)
		if exec == nil || a.Disabled || !expiresSoon(a, time.Now()) {
			continue
		}
		updated, errRefresh := exec.Refresh(ctx, a)
		if errRefresh != nil {
			fmt.Printf("%s: refresh failed: %v\n", a.FileName, errRefresh)
			code = 1
			continue
		}
		if _, errSave := store.Save(ctx, updated); errSave != nil {
			fmt.Printf("%s: save failed: %v\n", a.FileName, errSave)
			code = 1
			continue
		}
		fmt.Printf("%s: refreshed\n", a.FileName)
	}
	return code
}

// expiresSoon is whether the sign-in's access token expires within the lead.
// A sign-in with no recorded expiry is left alone: refreshing it on every call
// would rotate its token for nothing.
func expiresSoon(a *cliproxyauth.Auth, now time.Time) bool {
	s, _ := a.Metadata["expired"].(string)
	t, err := time.Parse(time.RFC3339, s)
	return err == nil && now.Add(refreshLead).After(t)
}
