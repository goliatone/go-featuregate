package urlkitadapter

import (
	"errors"

	"github.com/goliatone/go-featuregate/urlbuilder"
	"github.com/goliatone/go-urlkit"
)

// ErrResolverRequired indicates the urlkit resolver is missing.
var ErrResolverRequired = errors.New("urlkitadapter: resolver is required")

// Adapter wraps a urlkit.Resolver to satisfy urlbuilder.Builder.
type Adapter struct {
	Resolver urlkit.Resolver
}

// New builds a new Adapter for the provided resolver.
func New(resolver urlkit.Resolver) Adapter {
	return Adapter{Resolver: resolver}
}

// Resolve implements urlbuilder.Builder.
func (a Adapter) Resolve(groupPath, route string, params map[string]any, query map[string]string) (string, error) {
	if a.Resolver == nil {
		return "", ErrResolverRequired
	}
	return a.Resolver.Resolve(groupPath, route, params, query)
}

var _ urlbuilder.Builder = Adapter{}
