package activity

import (
	"context"

	"github.com/goliatone/go-featuregate/gate"
)

// Action describes a runtime override mutation.
type Action string

const (
	ActionSet   Action = "set"
	ActionUnset Action = "unset"
)

// UpdateEvent captures a runtime override mutation.
type UpdateEvent struct {
	Key           string
	NormalizedKey string
	Scope         gate.ScopeSet
	Actor         gate.ActorRef
	Action        Action
	Value         *bool
}

// Hook receives update events.
type Hook interface {
	OnUpdate(ctx context.Context, event UpdateEvent)
}

// HookFunc wraps a function as a Hook.
type HookFunc func(context.Context, UpdateEvent)

// OnUpdate implements Hook.
func (fn HookFunc) OnUpdate(ctx context.Context, event UpdateEvent) {
	if fn == nil {
		return
	}
	fn(ctx, event)
}

// NoopHook ignores updates.
type NoopHook struct{}

// OnUpdate implements Hook.
func (NoopHook) OnUpdate(context.Context, UpdateEvent) {}
