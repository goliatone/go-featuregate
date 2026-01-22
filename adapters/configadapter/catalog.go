package configadapter

import (
	"strings"

	"github.com/goliatone/go-featuregate/catalog"
	"github.com/goliatone/go-featuregate/gate"
)

// NewCatalog builds a feature catalog from a nested map.
func NewCatalog(data map[string]any, opts ...Option) *catalog.StaticCatalog {
	cfg := configOptions{delimiter: "."}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.delimiter == "" {
		cfg.delimiter = "."
	}

	defs := map[string]catalog.FeatureDefinition{}
	flattenCatalog("", data, cfg.delimiter, defs)
	return catalog.NewStatic(defs)
}

func flattenCatalog(prefix string, data map[string]any, delim string, out map[string]catalog.FeatureDefinition) {
	if len(data) == 0 {
		return
	}
	for key, value := range data {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		path := trimmedKey
		if prefix != "" {
			path = prefix + delim + trimmedKey
		}

		switch typed := value.(type) {
		case map[string]any:
			if def, ok := definitionFromMap(typed); ok {
				def.Key = path
				defsAdd(out, def)
				continue
			}
			flattenCatalog(path, typed, delim, out)
		case map[string]string:
			raw := map[string]any{}
			for k, v := range typed {
				raw[k] = v
			}
			if def, ok := definitionFromMap(raw); ok {
				def.Key = path
				defsAdd(out, def)
			}
		default:
			if msg, ok := messageFromValue(value); ok {
				defsAdd(out, catalog.FeatureDefinition{
					Key:         path,
					Description: msg,
				})
			}
		}
	}
}

func defsAdd(out map[string]catalog.FeatureDefinition, def catalog.FeatureDefinition) {
	normalized := gate.NormalizeKey(strings.TrimSpace(def.Key))
	if normalized == "" {
		return
	}
	def.Key = normalized
	out[normalized] = def
}

func definitionFromMap(data map[string]any) (catalog.FeatureDefinition, bool) {
	if msg, ok := messageFromValue(data["description"]); ok {
		return catalog.FeatureDefinition{Description: msg}, true
	}

	var msg catalog.Message
	if val, ok := data["description_key"].(string); ok && strings.TrimSpace(val) != "" {
		msg.Key = strings.TrimSpace(val)
	}
	if val, ok := data["description_text"].(string); ok && strings.TrimSpace(val) != "" {
		msg.Text = strings.TrimSpace(val)
	}
	if len(msg.Args) == 0 {
		msg.Args = nil
	}
	if msg.Key != "" || msg.Text != "" {
		return catalog.FeatureDefinition{Description: msg}, true
	}

	return catalog.FeatureDefinition{}, false
}

func messageFromValue(value any) (catalog.Message, bool) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return catalog.Message{}, false
		}
		return catalog.Message{Text: trimmed}, true
	case map[string]any:
		return messageFromMap(typed)
	case map[string]string:
		raw := map[string]any{}
		for key, val := range typed {
			raw[key] = val
		}
		return messageFromMap(raw)
	default:
		return catalog.Message{}, false
	}
}

func messageFromMap(data map[string]any) (catalog.Message, bool) {
	if len(data) == 0 {
		return catalog.Message{}, false
	}
	msg := catalog.Message{}
	if val, ok := data["key"].(string); ok {
		msg.Key = strings.TrimSpace(val)
	}
	if val, ok := data["text"].(string); ok {
		msg.Text = strings.TrimSpace(val)
	}
	if args, ok := data["args"].(map[string]any); ok && len(args) > 0 {
		msg.Args = args
	} else if args, ok := data["args"].(map[string]string); ok && len(args) > 0 {
		msg.Args = make(map[string]any, len(args))
		for key, val := range args {
			msg.Args[key] = val
		}
	}
	if msg.Key == "" && msg.Text == "" {
		return catalog.Message{}, false
	}
	if len(msg.Args) == 0 {
		msg.Args = nil
	}
	return msg, true
}
