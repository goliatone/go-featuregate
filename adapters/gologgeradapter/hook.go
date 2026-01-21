package gologgeradapter

import (
	"context"
	"strings"

	"github.com/goliatone/go-featuregate/activity"
	"github.com/goliatone/go-featuregate/gate"
	"github.com/goliatone/go-featuregate/scope"
	"github.com/goliatone/go-logger/glog"
)

// Hook logs resolve and update events using go-logger.
type Hook struct {
	logger         glog.Logger
	resolveLevel   string
	updateLevel    string
	resolveMessage string
	updateMessage  string
}

// Option customizes the logger hook.
type Option func(*Hook)

// New builds a logging hook for resolve/update events.
func New(logger glog.Logger, opts ...Option) *Hook {
	hook := &Hook{
		logger:         logger,
		resolveLevel:   "debug",
		updateLevel:    "info",
		resolveMessage: "featuregate.resolve",
		updateMessage:  "featuregate.update",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(hook)
		}
	}
	return hook
}

// WithResolveLevel sets the log level for resolve events.
func WithResolveLevel(level string) Option {
	return func(hook *Hook) {
		if hook == nil {
			return
		}
		hook.resolveLevel = strings.ToLower(strings.TrimSpace(level))
	}
}

// WithUpdateLevel sets the log level for update events.
func WithUpdateLevel(level string) Option {
	return func(hook *Hook) {
		if hook == nil {
			return
		}
		hook.updateLevel = strings.ToLower(strings.TrimSpace(level))
	}
}

// WithResolveMessage overrides the resolve log message.
func WithResolveMessage(message string) Option {
	return func(hook *Hook) {
		if hook == nil {
			return
		}
		hook.resolveMessage = message
	}
}

// WithUpdateMessage overrides the update log message.
func WithUpdateMessage(message string) Option {
	return func(hook *Hook) {
		if hook == nil {
			return
		}
		hook.updateMessage = message
	}
}

// OnResolve implements gate.ResolveHook.
func (h *Hook) OnResolve(ctx context.Context, event gate.ResolveEvent) {
	if h == nil || h.logger == nil {
		return
	}
	fields := map[string]any{
		"feature_key":         event.Key,
		"feature_key_norm":    event.NormalizedKey,
		"feature_value":       event.Value,
		"feature_source":      event.Source,
		"feature_cache_hit":   event.Trace.CacheHit,
		"feature_override":    event.Trace.Override.State,
		"feature_default_set": event.Trace.Default.Set,
	}
	if event.Error != nil {
		fields["feature_error"] = event.Error.Error()
	}
	for key, value := range scopeFields(event.Scope) {
		fields[key] = value
	}
	h.log(ctx, h.resolveLevel, h.resolveMessage, fields)
}

// OnUpdate implements activity.Hook.
func (h *Hook) OnUpdate(ctx context.Context, event activity.UpdateEvent) {
	if h == nil || h.logger == nil {
		return
	}
	fields := map[string]any{
		"feature_key":      event.Key,
		"feature_key_norm": event.NormalizedKey,
		"feature_action":   event.Action,
		"feature_value":    event.Value,
		"actor_id":         event.Actor.ID,
		"actor_type":       event.Actor.Type,
		"actor_name":       event.Actor.Name,
	}
	for key, value := range scopeFields(event.Scope) {
		fields[key] = value
	}
	h.log(ctx, h.updateLevel, h.updateMessage, fields)
}

func (h *Hook) log(ctx context.Context, level string, message string, fields map[string]any) {
	logger := h.logger
	if logger == nil {
		return
	}
	if ctx != nil {
		logger = logger.WithContext(ctx)
	}
	if fieldsLogger, ok := logger.(glog.FieldsLogger); ok && len(fields) > 0 {
		logger = fieldsLogger.WithFields(fields)
	}
	switch level {
	case "trace":
		logger.Trace(message)
	case "debug":
		logger.Debug(message)
	case "warn":
		logger.Warn(message)
	case "error":
		logger.Error(message)
	case "fatal":
		// Avoid Fatal in go-featuregate; treat fatal as error instead.
		logger.Error(message)
	default:
		logger.Info(message)
	}
}

func scopeFields(scopeSet gate.ScopeSet) map[string]any {
	return map[string]any{
		scope.MetadataTenantID: scopeSet.TenantID,
		scope.MetadataOrgID:    scopeSet.OrgID,
		scope.MetadataUserID:   scopeSet.UserID,
		"system":               scopeSet.System,
	}
}

var _ gate.ResolveHook = (*Hook)(nil)
var _ activity.Hook = (*Hook)(nil)
