package main

import (
	"context"
	"fmt"

	"github.com/goliatone/go-featuregate/adapters/configadapter"
	fggate "github.com/goliatone/go-featuregate/gate"
	"github.com/goliatone/go-featuregate/resolver"
	"github.com/goliatone/go-featuregate/store"
)

func main() {
	defaults := configadapter.NewDefaultsFromBools(map[string]bool{
		"dashboard": true,
	})
	overrides := store.NewMemoryStore()

	featureGate := resolver.New(
		resolver.WithDefaults(defaults),
		resolver.WithOverrideStore(overrides),
	)

	ctx := context.Background()
	scope := fggate.ScopeSet{TenantID: "acme"}
	actor := fggate.ActorRef{ID: "admin-1", Type: "user"}

	value, err := featureGate.Enabled(ctx, "dashboard", fggate.WithScopeSet(scope))
	if err != nil {
		fmt.Println("default resolve error:", err)
		return
	}
	fmt.Println("dashboard default:", value)

	if err := featureGate.Set(ctx, "dashboard", scope, false, actor); err != nil {
		fmt.Println("set override error:", err)
		return
	}
	value, err = featureGate.Enabled(ctx, "dashboard", fggate.WithScopeSet(scope))
	if err != nil {
		fmt.Println("override resolve error:", err)
		return
	}
	fmt.Println("dashboard override:", value)

	if err := featureGate.Unset(ctx, "dashboard", scope, actor); err != nil {
		fmt.Println("unset override error:", err)
		return
	}
	value, err = featureGate.Enabled(ctx, "dashboard", fggate.WithScopeSet(scope))
	if err != nil {
		fmt.Println("unset resolve error:", err)
		return
	}
	fmt.Println("dashboard unset:", value)
}
