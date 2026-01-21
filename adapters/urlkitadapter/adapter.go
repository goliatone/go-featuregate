package urlkitadapter

import (
	"github.com/goliatone/go-featuregate/ferrors"
	"github.com/goliatone/go-featuregate/urlbuilder"
	"github.com/goliatone/go-urlkit"
)

// ErrResolverRequired indicates the urlkit resolver is missing.
var ErrResolverRequired = ferrors.ErrResolverRequired

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
		return "", ferrors.WrapSentinel(ferrors.ErrResolverRequired, "urlkitadapter: resolver is required", map[string]any{
			ferrors.MetaAdapter:   "urlkit",
			ferrors.MetaOperation: "resolve",
		})
	}
	url, err := a.Resolver.Resolve(groupPath, route, params, query)
	if err != nil {
		return "", ferrors.WrapExternal(err, ferrors.TextCodeAdapterFailed, "urlkitadapter: resolve failed", map[string]any{
			ferrors.MetaAdapter:   "urlkit",
			ferrors.MetaOperation: "resolve",
		})
	}
	return url, nil
}

var _ urlbuilder.Builder = Adapter{}
