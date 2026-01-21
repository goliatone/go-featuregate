package optionsadapter

import (
	"strings"

	"github.com/goliatone/go-featuregate/ferrors"
)

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

func lookupPath(snapshot map[string]any, path string) (any, bool) {
	if len(snapshot) == 0 {
		return nil, false
	}
	if value, ok := snapshot[path]; ok {
		return value, true
	}
	segments := splitPath(path)
	if len(segments) == 0 {
		return nil, false
	}
	var current any = snapshot
	for _, segment := range segments {
		nextMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := nextMap[segment]
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}

func setPath(snapshot map[string]any, path string, value any) error {
	segments := splitPath(path)
	if len(segments) == 0 {
		return ferrors.WrapSentinel(ferrors.ErrPathRequired, "optionsadapter: path is empty", map[string]any{
			ferrors.MetaPath: path,
		})
	}
	current := snapshot
	for _, segment := range segments[:len(segments)-1] {
		next, ok := current[segment]
		if !ok {
			child := map[string]any{}
			current[segment] = child
			current = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return ferrors.WrapSentinel(ferrors.ErrPathInvalid, "optionsadapter: path segment is not a map", map[string]any{
				ferrors.MetaPath: segment,
			})
		}
		current = child
	}
	current[segments[len(segments)-1]] = value
	return nil
}

func deletePath(snapshot map[string]any, path string) bool {
	segments := splitPath(path)
	if len(segments) == 0 || len(snapshot) == 0 {
		return false
	}
	var (
		nodes []map[string]any
		keys  []string
	)
	current := snapshot
	for _, segment := range segments {
		nodes = append(nodes, current)
		keys = append(keys, segment)
		if len(keys) == len(segments) {
			break
		}
		next, ok := current[segment].(map[string]any)
		if !ok {
			return false
		}
		current = next
	}

	last := nodes[len(nodes)-1]
	if _, ok := last[keys[len(keys)-1]]; !ok {
		return false
	}
	delete(last, keys[len(keys)-1])

	for i := len(nodes) - 1; i > 0; i-- {
		if len(nodes[i]) != 0 {
			break
		}
		delete(nodes[i-1], keys[i-1])
	}
	return true
}

func flattenMap(prefix string, data map[string]any, out map[string]any) {
	if len(data) == 0 {
		return
	}
	for key, value := range data {
		if key == "" {
			continue
		}
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		child, ok := value.(map[string]any)
		if ok {
			flattenMap(path, child, out)
			continue
		}
		out[path] = value
	}
}
