package catalog

import (
	"context"
	"sort"
	"strings"

	"github.com/goliatone/go-featuregate/gate"
)

// Message represents a human-friendly string with optional localization data.
type Message struct {
	Key  string
	Text string
	Args map[string]any
}

// FeatureDefinition describes a feature flag for UI and documentation.
type FeatureDefinition struct {
	Key         string
	Description Message
}

// Catalog exposes feature definitions by key.
type Catalog interface {
	Get(key string) (FeatureDefinition, bool)
	List() []FeatureDefinition
}

// MessageResolver resolves a Message to a display string.
type MessageResolver interface {
	Resolve(ctx context.Context, locale string, msg Message) (string, error)
}

// PlainResolver returns the Message text or key without localization.
type PlainResolver struct{}

// Resolve implements MessageResolver.
func (PlainResolver) Resolve(_ context.Context, _ string, msg Message) (string, error) {
	if msg.Text != "" {
		return msg.Text, nil
	}
	return msg.Key, nil
}

// StaticCatalog provides an in-memory catalog.
type StaticCatalog struct {
	defs map[string]FeatureDefinition
}

// NewStatic builds an in-memory catalog from provided definitions.
func NewStatic(defs map[string]FeatureDefinition) *StaticCatalog {
	out := make(map[string]FeatureDefinition, len(defs))
	for key, def := range defs {
		normalized := gate.NormalizeKey(strings.TrimSpace(key))
		if normalized == "" {
			continue
		}
		def.Key = normalized
		def.Description = normalizeMessage(def.Description)
		out[normalized] = def
	}
	return &StaticCatalog{defs: out}
}

// Get implements Catalog.
func (c *StaticCatalog) Get(key string) (FeatureDefinition, bool) {
	if c == nil || len(c.defs) == 0 {
		return FeatureDefinition{}, false
	}
	normalized := gate.NormalizeKey(strings.TrimSpace(key))
	if normalized == "" {
		return FeatureDefinition{}, false
	}
	def, ok := c.defs[normalized]
	return def, ok
}

// List implements Catalog.
func (c *StaticCatalog) List() []FeatureDefinition {
	if c == nil || len(c.defs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(c.defs))
	for key := range c.defs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]FeatureDefinition, 0, len(keys))
	for _, key := range keys {
		out = append(out, c.defs[key])
	}
	return out
}

func normalizeMessage(msg Message) Message {
	msg.Key = strings.TrimSpace(msg.Key)
	msg.Text = strings.TrimSpace(msg.Text)
	if len(msg.Args) == 0 {
		msg.Args = nil
	}
	return msg
}
