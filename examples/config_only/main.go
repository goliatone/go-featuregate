package main

import (
	"context"
	"fmt"

	"github.com/goliatone/go-config/config"
	"github.com/goliatone/go-featuregate/adapters/configadapter"
	"github.com/goliatone/go-featuregate/resolver"
)

func main() {
	defaults := configadapter.NewDefaults(map[string]any{
		"users": map[string]any{
			"signup": config.NewOptionalBool(true),
		},
		"dashboard": config.NewOptionalBool(false),
	})

	gate := resolver.New(
		resolver.WithDefaults(defaults),
	)

	ctx := context.Background()
	usersSignup, err := gate.Enabled(ctx, "users.signup")
	if err != nil {
		fmt.Println("users.signup error:", err)
		return
	}
	dashboard, err := gate.Enabled(ctx, "dashboard")
	if err != nil {
		fmt.Println("dashboard error:", err)
		return
	}

	fmt.Println("users.signup:", usersSignup)
	fmt.Println("dashboard:", dashboard)
}
