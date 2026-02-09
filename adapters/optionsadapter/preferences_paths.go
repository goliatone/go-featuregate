package optionsadapter

import (
	"strconv"
	"strings"

	"github.com/goliatone/go-featuregate/ferrors"
)

func flattenSnapshot(snapshot map[string]any, allow map[string]struct{}) (map[string]any, error) {
	out := map[string]any{}
	if len(snapshot) == 0 {
		return out, nil
	}
	for key, value := range snapshot {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if strings.Contains(key, ".") {
			return nil, pathInvalidError(key, "map key cannot contain dot")
		}
		if err := flattenValue(key, value, out, allow); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func flattenValue(path string, value any, out map[string]any, allow map[string]struct{}) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if strings.Contains(key, ".") {
				return pathInvalidError(joinPath(path, key), "map key cannot contain dot")
			}
			if err := flattenValue(joinPath(path, key), child, out, allow); err != nil {
				return err
			}
		}
	case []any:
		for idx, child := range typed {
			if err := flattenValue(joinPath(path, strconv.Itoa(idx)), child, out, allow); err != nil {
				return err
			}
		}
	default:
		if path == "" {
			return pathInvalidError(path, "path is required")
		}
		if len(allow) > 0 {
			if _, ok := allow[path]; !ok {
				return nil
			}
		}
		out[path] = value
	}
	return nil
}

func setPathStrict(snapshot map[string]any, path string, value any) error {
	segments, err := parsePath(path)
	if err != nil {
		return err
	}
	updated, err := assignPath(snapshot, segments, value)
	if err != nil {
		return err
	}
	if _, ok := updated.(map[string]any); !ok {
		return pathInvalidError(path, "root must be a map")
	}
	return nil
}

func unflattenSnapshot(flat map[string]any) (map[string]any, error) {
	out := map[string]any{}
	for path, value := range flat {
		if err := setPathStrict(out, path, value); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func assignPath(node any, segments []string, value any) (any, error) {
	if len(segments) == 0 {
		return nil, pathInvalidError("", "path is required")
	}
	head := segments[0]
	last := len(segments) == 1

	switch typed := node.(type) {
	case map[string]any:
		if head == "" {
			return nil, pathInvalidError(head, "empty path segment")
		}
		if last {
			typed[head] = value
			return typed, nil
		}
		child, ok := typed[head]
		if !ok || child == nil {
			child = containerFor(segments[1])
		}
		updatedChild, err := assignPath(child, segments[1:], value)
		if err != nil {
			return nil, err
		}
		typed[head] = updatedChild
		return typed, nil
	case []any:
		index, err := parseIndex(head)
		if err != nil {
			return nil, pathInvalidError(head, "array path segment must be a non-negative integer")
		}
		if index >= len(typed) {
			grown := make([]any, index+1)
			copy(grown, typed)
			typed = grown
		}
		if last {
			typed[index] = value
			return typed, nil
		}
		child := typed[index]
		if child == nil {
			child = containerFor(segments[1])
		}
		updatedChild, err := assignPath(child, segments[1:], value)
		if err != nil {
			return nil, err
		}
		typed[index] = updatedChild
		return typed, nil
	default:
		return nil, pathInvalidError(strings.Join(segments, "."), "path segment is not traversable")
	}
}

func parsePath(path string) ([]string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, pathInvalidError(path, "path is required")
	}
	parts := strings.Split(path, ".")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, pathInvalidError(path, "path contains empty segment")
		}
		out = append(out, part)
	}
	return out, nil
}

func parseIndex(segment string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(segment))
}

func containerFor(next string) any {
	if _, err := parseIndex(next); err == nil {
		return []any{}
	}
	return map[string]any{}
}

func joinPath(left, right string) string {
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	return left + "." + right
}

func pathInvalidError(path, reason string) error {
	meta := map[string]any{
		ferrors.MetaPath: path,
	}
	if reason != "" {
		meta["reason"] = reason
	}
	return ferrors.WrapSentinel(
		ErrPreferencesPathInvalid,
		"optionsadapter: invalid preferences path",
		meta,
	)
}
