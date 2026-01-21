package templates

import (
	"context"
	"fmt"
	"strings"

	"github.com/flosch/pongo2/v6"

	"github.com/goliatone/go-featuregate/ferrors"
	"github.com/goliatone/go-featuregate/gate"
	"github.com/goliatone/go-featuregate/logger"
	"github.com/goliatone/go-featuregate/scope"
)

const (
	TemplateContextKey  = "feature_ctx"
	TemplateScopeKey    = "feature_scope"
	TemplateSnapshotKey = "feature_snapshot"
)

// HelperConfig configures template helpers.
type HelperConfig struct {
	ContextKey             string
	ScopeKey               string
	SnapshotKey            string
	EnableStructuredErrors bool
	EnableErrorLogging     bool
	Logger                 logger.Logger
}

// HelperOption configures template helpers.
type HelperOption func(*HelperConfig)

// DefaultHelperConfig returns the default helper configuration.
func DefaultHelperConfig() HelperConfig {
	return HelperConfig{
		ContextKey:             TemplateContextKey,
		ScopeKey:               TemplateScopeKey,
		SnapshotKey:            TemplateSnapshotKey,
		EnableStructuredErrors: false,
		EnableErrorLogging:     false,
	}
}

// WithContextKey overrides the template context key name.
func WithContextKey(key string) HelperOption {
	return func(cfg *HelperConfig) {
		if cfg == nil {
			return
		}
		cfg.ContextKey = strings.TrimSpace(key)
	}
}

// WithScopeKey overrides the template scope key name.
func WithScopeKey(key string) HelperOption {
	return func(cfg *HelperConfig) {
		if cfg == nil {
			return
		}
		cfg.ScopeKey = strings.TrimSpace(key)
	}
}

// WithSnapshotKey overrides the template snapshot key name.
func WithSnapshotKey(key string) HelperOption {
	return func(cfg *HelperConfig) {
		if cfg == nil {
			return
		}
		cfg.SnapshotKey = strings.TrimSpace(key)
	}
}

// WithStructuredErrors toggles structured error output for string helpers.
func WithStructuredErrors(enabled bool) HelperOption {
	return func(cfg *HelperConfig) {
		if cfg == nil {
			return
		}
		cfg.EnableStructuredErrors = enabled
	}
}

// WithErrorLogging toggles error logging for helper failures.
func WithErrorLogging(enabled bool) HelperOption {
	return func(cfg *HelperConfig) {
		if cfg == nil {
			return
		}
		cfg.EnableErrorLogging = enabled
	}
}

// WithLogger injects a logger for helper error logging.
func WithLogger(lgr logger.Logger) HelperOption {
	return func(cfg *HelperConfig) {
		if cfg == nil {
			return
		}
		cfg.Logger = lgr
	}
}

// TemplateHelpers returns a helper set suitable for WithTemplateFunc.
func TemplateHelpers(featureGate gate.FeatureGate, opts ...HelperOption) map[string]any {
	cfg := DefaultHelperConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.EnableErrorLogging && cfg.Logger == nil {
		cfg.Logger = logger.Default()
	}
	helpers := &helperSet{
		gate:  featureGate,
		trace: traceGate(featureGate),
		cfg:   cfg,
	}

	funcs := map[string]any{
		"feature":       helpers.feature,
		"feature_any":   helpers.featureAny,
		"feature_all":   helpers.featureAll,
		"feature_none":  helpers.featureNone,
		"feature_if":    helpers.featureIf,
		"feature_class": helpers.featureClass,
	}
	if helpers.trace != nil {
		funcs["feature_trace"] = helpers.featureTrace
	}
	return funcs
}

type helperSet struct {
	gate  gate.FeatureGate
	trace gate.TraceableFeatureGate
	cfg   HelperConfig
}

func (h *helperSet) feature(execCtx *pongo2.ExecutionContext, key any) bool {
	normalized, ok := parseKey(key)
	if !ok {
		return false
	}
	value, err := h.resolveValue(execCtx, normalized)
	if err != nil {
		return false
	}
	return value
}

func (h *helperSet) featureAny(execCtx *pongo2.ExecutionContext, keys ...any) bool {
	parsed := parseKeys(keys...)
	if len(parsed) == 0 {
		return false
	}
	for _, key := range parsed {
		value, err := h.resolveValue(execCtx, key)
		if err == nil && value {
			return true
		}
	}
	return false
}

func (h *helperSet) featureAll(execCtx *pongo2.ExecutionContext, keys ...any) bool {
	parsed := parseKeys(keys...)
	if len(parsed) == 0 {
		return false
	}
	for _, key := range parsed {
		value, err := h.resolveValue(execCtx, key)
		if err != nil || !value {
			return false
		}
	}
	return true
}

func (h *helperSet) featureNone(execCtx *pongo2.ExecutionContext, keys ...any) bool {
	parsed := parseKeys(keys...)
	if len(parsed) == 0 {
		return false
	}
	for _, key := range parsed {
		value, err := h.resolveValue(execCtx, key)
		if err == nil && value {
			return false
		}
	}
	return true
}

func (h *helperSet) featureIf(execCtx *pongo2.ExecutionContext, key any, whenTrue any, whenFalse ...any) any {
	var fallback any = ""
	if len(whenFalse) > 0 {
		fallback = whenFalse[0]
	}
	normalized, ok := parseKey(key)
	if !ok {
		return h.errorOrFallback("feature_if", ferrors.WrapSentinel(ferrors.ErrInvalidKey, "feature key is required", map[string]any{
			ferrors.MetaFeatureKey: key,
		}), fallback)
	}
	value, err := h.resolveValue(execCtx, normalized)
	if err != nil {
		return h.errorOrFallback("feature_if", err, fallback)
	}
	if value {
		return whenTrue
	}
	return fallback
}

func (h *helperSet) featureClass(execCtx *pongo2.ExecutionContext, key any, on any, off ...any) any {
	var fallback any = ""
	if len(off) > 0 {
		fallback = off[0]
	}
	normalized, ok := parseKey(key)
	if !ok {
		return h.errorOrFallback("feature_class", ferrors.WrapSentinel(ferrors.ErrInvalidKey, "feature key is required", map[string]any{
			ferrors.MetaFeatureKey: key,
		}), fallback)
	}
	value, err := h.resolveValue(execCtx, normalized)
	if err != nil {
		return h.errorOrFallback("feature_class", err, fallback)
	}
	if value {
		return on
	}
	return fallback
}

func (h *helperSet) featureTrace(execCtx *pongo2.ExecutionContext, key any) any {
	normalized, ok := parseKey(key)
	if !ok {
		return h.errorOrFallback("feature_trace", ferrors.WrapSentinel(ferrors.ErrInvalidKey, "feature key is required", map[string]any{
			ferrors.MetaFeatureKey: key,
		}), nil)
	}
	if snapshot := h.snapshot(execCtx); snapshot != nil {
		if trace, ok := snapshotTrace(snapshot, normalized); ok {
			return trace
		}
	}
	if h.trace == nil {
		return nil
	}

	ctx := h.context(execCtx)
	opts := h.resolveOptions(execCtx)
	_, trace, err := h.trace.ResolveWithTrace(ctx, normalized, opts...)
	if err != nil {
		return h.errorOrFallback("feature_trace", err, nil)
	}
	return trace
}

func (h *helperSet) resolveValue(execCtx *pongo2.ExecutionContext, key string) (bool, error) {
	if key == "" {
		return false, ferrors.WrapSentinel(ferrors.ErrInvalidKey, "feature key is required", map[string]any{
			ferrors.MetaFeatureKey: key,
		})
	}
	if snapshot := h.snapshot(execCtx); snapshot != nil {
		if value, ok := snapshotValue(snapshot, key); ok {
			return value, nil
		}
	}
	if h.gate == nil {
		return false, ferrors.WrapSentinel(ferrors.ErrGateRequired, "feature gate is required", nil)
	}
	ctx := h.context(execCtx)
	opts := h.resolveOptions(execCtx)
	return h.gate.Enabled(ctx, key, opts...)
}

func (h *helperSet) resolveOptions(execCtx *pongo2.ExecutionContext) []gate.ResolveOption {
	if scopeSet := h.scope(execCtx); scopeSet != nil {
		return []gate.ResolveOption{gate.WithScopeSet(*scopeSet)}
	}
	return nil
}

func (h *helperSet) context(execCtx *pongo2.ExecutionContext) context.Context {
	data := templateData(execCtx)
	if data == nil {
		return context.Background()
	}
	key := h.cfg.ContextKey
	if key == "" {
		key = TemplateContextKey
	}
	raw, ok := data[key]
	if !ok || raw == nil {
		return context.Background()
	}
	return contextFromValue(raw)
}

func (h *helperSet) scope(execCtx *pongo2.ExecutionContext) *gate.ScopeSet {
	data := templateData(execCtx)
	if data == nil {
		return nil
	}
	key := h.cfg.ScopeKey
	if key == "" {
		key = TemplateScopeKey
	}
	raw, ok := data[key]
	if !ok || raw == nil {
		return nil
	}
	scopeSet, ok := scopeFromValue(raw)
	if !ok {
		return nil
	}
	return &scopeSet
}

func (h *helperSet) snapshot(execCtx *pongo2.ExecutionContext) any {
	data := templateData(execCtx)
	if data == nil {
		return nil
	}
	key := h.cfg.SnapshotKey
	if key == "" {
		key = TemplateSnapshotKey
	}
	raw, ok := data[key]
	if !ok {
		return nil
	}
	return raw
}

func (h *helperSet) errorOrFallback(helper string, err error, fallback any) any {
	if h.cfg.EnableStructuredErrors {
		if h.cfg.EnableErrorLogging {
			h.logHelperError(helper, err)
		}
		return templateError(helper, err)
	}
	if h.cfg.EnableErrorLogging {
		h.logHelperError(helper, err)
	}
	return fallback
}

// TemplateError provides structured helper error output.
type TemplateError struct {
	Helper   string         `json:"helper"`
	Type     string         `json:"type,omitempty"`
	Message  string         `json:"message,omitempty"`
	Category string         `json:"category,omitempty"`
	TextCode string         `json:"text_code,omitempty"`
	Context  map[string]any `json:"context,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func templateError(helper string, err error) TemplateError {
	out := TemplateError{Helper: helper}
	if err == nil {
		return out
	}
	if rich, ok := ferrors.As(err); ok {
		out.Message = rich.Message
		out.Category = rich.Category.String()
		out.TextCode = rich.TextCode
		if len(rich.Metadata) > 0 {
			out.Metadata = rich.Metadata
			out.Context = rich.Metadata
		}
		if out.TextCode != "" {
			out.Type = out.TextCode
		} else if out.Category != "" {
			out.Type = out.Category
		}
		return out
	}
	out.Message = err.Error()
	out.Type = "error"
	return out
}

// SnapshotReader reports stored feature values by key.
type SnapshotReader interface {
	Enabled(key string) (bool, bool)
}

// TraceSnapshotReader exposes trace data for feature keys.
type TraceSnapshotReader interface {
	SnapshotReader
	Trace(key string) (gate.ResolveTrace, bool)
}

// Snapshot holds optional precomputed values and traces.
type Snapshot struct {
	Values map[string]bool
	Traces map[string]gate.ResolveTrace
}

// Enabled implements SnapshotReader.
func (s Snapshot) Enabled(key string) (bool, bool) {
	key = gate.NormalizeKey(strings.TrimSpace(key))
	if key == "" {
		return false, false
	}
	if value, ok := s.Values[key]; ok {
		return value, true
	}
	return false, false
}

// Trace implements TraceSnapshotReader.
func (s Snapshot) Trace(key string) (gate.ResolveTrace, bool) {
	key = gate.NormalizeKey(strings.TrimSpace(key))
	if key == "" {
		return gate.ResolveTrace{}, false
	}
	trace, ok := s.Traces[key]
	return trace, ok
}

func snapshotValue(snapshot any, key string) (bool, bool) {
	if reader, ok := snapshot.(SnapshotReader); ok {
		return reader.Enabled(key)
	}
	switch typed := snapshot.(type) {
	case map[string]bool:
		value, ok := typed[key]
		return value, ok
	case map[string]gate.ResolveTrace:
		trace, ok := typed[key]
		return trace.Value, ok
	case map[string]any:
		if value, ok := typed[key]; ok {
			return boolFromValue(value)
		}
		if value, ok := lookupNestedValue(typed, key); ok {
			return boolFromValue(value)
		}
	}
	return false, false
}

func snapshotTrace(snapshot any, key string) (gate.ResolveTrace, bool) {
	if reader, ok := snapshot.(TraceSnapshotReader); ok {
		return reader.Trace(key)
	}
	switch typed := snapshot.(type) {
	case map[string]gate.ResolveTrace:
		trace, ok := typed[key]
		return trace, ok
	case map[string]*gate.ResolveTrace:
		trace, ok := typed[key]
		if !ok || trace == nil {
			return gate.ResolveTrace{}, false
		}
		return *trace, true
	}
	return gate.ResolveTrace{}, false
}

func boolFromValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case *bool:
		if typed == nil {
			return false, false
		}
		return *typed, true
	default:
		return false, false
	}
}

func lookupNestedValue(snapshot map[string]any, key string) (any, bool) {
	if len(snapshot) == 0 {
		return nil, false
	}
	parts := splitPath(key)
	if len(parts) == 0 {
		return nil, false
	}
	var current any = snapshot
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := m[part]
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}

func parseKey(value any) (string, bool) {
	raw := unwrapValue(value)
	switch typed := raw.(type) {
	case string:
		normalized := gate.NormalizeKey(strings.TrimSpace(typed))
		return normalized, normalized != ""
	case fmt.Stringer:
		normalized := gate.NormalizeKey(strings.TrimSpace(typed.String()))
		return normalized, normalized != ""
	default:
		return "", false
	}
}

func parseKeys(values ...any) []string {
	keys := make([]string, 0, len(values))
	for _, value := range values {
		for _, key := range flattenKeys(value) {
			if normalized, ok := parseKey(key); ok {
				keys = append(keys, normalized)
			}
		}
	}
	return keys
}

func flattenKeys(value any) []any {
	value = unwrapValue(value)
	switch typed := value.(type) {
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case []any:
		return typed
	default:
		return []any{value}
	}
}

func unwrapValue(value any) any {
	if value == nil {
		return nil
	}
	if pv, ok := value.(*pongo2.Value); ok && pv != nil {
		return pv.Interface()
	}
	return value
}

func contextFromValue(value any) context.Context {
	switch typed := value.(type) {
	case context.Context:
		return typed
	case interface{ Context() context.Context }:
		return typed.Context()
	default:
		return context.Background()
	}
}

func scopeFromValue(value any) (gate.ScopeSet, bool) {
	switch typed := value.(type) {
	case gate.ScopeSet:
		return typed, true
	case *gate.ScopeSet:
		if typed == nil {
			return gate.ScopeSet{}, false
		}
		return *typed, true
	case map[string]any:
		return scopeFromMap(typed)
	case map[string]string:
		raw := map[string]any{}
		for key, val := range typed {
			raw[key] = val
		}
		return scopeFromMap(raw)
	default:
		return gate.ScopeSet{}, false
	}
}

func scopeFromMap(data map[string]any) (gate.ScopeSet, bool) {
	if len(data) == 0 {
		return gate.ScopeSet{}, false
	}
	scopeSet := gate.ScopeSet{}
	if val, ok := data[scope.MetadataTenantID]; ok {
		scopeSet.TenantID, _ = val.(string)
	}
	if val, ok := data[scope.MetadataOrgID]; ok {
		scopeSet.OrgID, _ = val.(string)
	}
	if val, ok := data[scope.MetadataUserID]; ok {
		scopeSet.UserID, _ = val.(string)
	}
	if val, ok := data["system"]; ok {
		if flag, ok := val.(bool); ok {
			scopeSet.System = flag
		}
	}
	if scopeSet == (gate.ScopeSet{}) {
		return gate.ScopeSet{}, false
	}
	return scopeSet, true
}

func templateData(execCtx *pongo2.ExecutionContext) map[string]any {
	if execCtx == nil || execCtx.Public == nil {
		return nil
	}
	data := make(map[string]any, len(execCtx.Public))
	for key, value := range execCtx.Public {
		data[key] = value
	}
	return data
}

func splitPath(path string) []string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ".")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func (h *helperSet) logHelperError(helper string, err error) {
	if h == nil || h.cfg.Logger == nil {
		return
	}
	args := []any{
		"helper", helper,
		"error", err,
	}
	if rich, ok := ferrors.As(err); ok {
		args = append(args,
			"category", rich.Category,
			"text_code", rich.TextCode,
			"metadata", rich.Metadata,
		)
	}
	h.cfg.Logger.Error("featuregate.helper_error", args...)
}

func traceGate(featureGate gate.FeatureGate) gate.TraceableFeatureGate {
	if featureGate == nil {
		return nil
	}
	traceable, ok := featureGate.(gate.TraceableFeatureGate)
	if !ok {
		return nil
	}
	return traceable
}
