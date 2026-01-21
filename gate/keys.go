package gate

import (
	"sort"
	"strings"
)

const (
	FeatureUsersSignup           = "users.signup"
	FeatureUsersSelfRegistration = "users.self_registration"
)

var keyAliases = map[string]string{
	FeatureUsersSelfRegistration: FeatureUsersSignup,
}

// NormalizeKey trims whitespace and resolves any known aliases.
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

// AliasesFor returns the legacy alias keys for the provided key.
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
