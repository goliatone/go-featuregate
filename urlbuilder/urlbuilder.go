package urlbuilder

// Builder resolves group/route pairs into URLs.
type Builder interface {
	Resolve(groupPath, route string, params map[string]any, query map[string]string) (string, error)
}
