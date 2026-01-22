package gate

import (
	"sort"
	"strings"
)

const (
	FeatureUsersSignup        = "users.signup"
	FeatureUsersPasswordReset = "users.password_reset"
	// FeatureUsersPasswordResetFinalize duplicates the go-auth string (go-auth owns the literal).
	FeatureUsersPasswordResetFinalize = "users.password_reset.finalize"
)

var keyAliases = map[string]string{} // Legacy aliases are intentionally disabled.

// NormalizeKey trims whitespace and resolves any known aliases (if configured).
func NormalizeKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if alias, ok := keyAliases[key]; ok {
		return alias
	}
	return key
}

// ResolveAlias returns the normalized key and whether an alias was applied.
func ResolveAlias(key string) (string, bool) {
	normalized := NormalizeKey(key)
	if normalized == "" {
		return "", false
	}
	return normalized, normalized != strings.TrimSpace(key)
}

// IsAlias reports whether the key is a known alias.
func IsAlias(key string) bool {
	_, ok := keyAliases[strings.TrimSpace(key)]
	return ok
}

// AliasesFor returns the legacy alias keys for the provided key, if any.
func AliasesFor(key string) []string {
	normalized := NormalizeKey(key)
	if normalized == "" {
		return nil
	}
	aliases := make([]string, 0, len(keyAliases))
	for alias, canonical := range keyAliases {
		if canonical == normalized {
			aliases = append(aliases, alias)
		}
	}
	if len(aliases) == 0 {
		return nil
	}
	sort.Strings(aliases)
	return aliases
}
