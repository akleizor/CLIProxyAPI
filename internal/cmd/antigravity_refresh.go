package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/runtime/executor"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
	sdkAuth "github.com/router-for-me/CLIProxyAPI/v8/sdk/auth"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

// antigravityRefreshLead refreshes a sign-in this close to expiry, so a caller
// that reads the token right after gets one that outlives its request.
const antigravityRefreshLead = 10 * time.Minute

// DoAntigravityRefresh refreshes every Antigravity sign-in in the auth dir that
// expires soon and saves it, then exits. It lets a tool that only reads the
// token (ccmorph's quota reader) keep the sign-in alive with no proxy running.
func DoAntigravityRefresh(cfg *config.Config) int {
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
	exec := executor.NewAntigravityExecutor(cfg)
	code := 0
	for _, a := range auths {
		if a.Provider != "antigravity" || a.Disabled || !antigravityExpiresSoon(a, time.Now()) {
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

// antigravityExpiresSoon is whether the sign-in's access token expires within
// the lead; a missing or unreadable expiry counts as expiring.
func antigravityExpiresSoon(a *cliproxyauth.Auth, now time.Time) bool {
	s, _ := a.Metadata["expired"].(string)
	t, err := time.Parse(time.RFC3339, s)
	return err != nil || now.Add(antigravityRefreshLead).After(t)
}
