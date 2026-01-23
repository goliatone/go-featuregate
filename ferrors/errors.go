package ferrors

import (
	goerrors "github.com/goliatone/go-errors"
)

const (
	MetaFeatureKey           = "feature_key"
	MetaFeatureKeyNormalized = "feature_key_norm"
	MetaScope                = "scope"
	MetaChain                = "scope_chain"
	MetaStore                = "store"
	MetaAdapter              = "adapter"
	MetaDomain               = "domain"
	MetaTable                = "table"
	MetaOperation            = "operation"
	MetaStrict               = "strict"
	MetaPath                 = "path"
)

const (
	TextCodeInvalidKey               = "FEATURE_KEY_REQUIRED"
	TextCodeStoreUnavailable         = "OVERRIDE_STORE_REQUIRED"
	TextCodeStoreRequired            = "STORE_REQUIRED"
	TextCodeResolverRequired         = "RESOLVER_REQUIRED"
	TextCodeGateRequired             = "FEATURE_GATE_REQUIRED"
	TextCodeScopeRequired            = "SCOPE_REQUIRED"
	TextCodeSnapshotRequired         = "SNAPSHOT_REQUIRED"
	TextCodePathRequired             = "PATH_REQUIRED"
	TextCodePathInvalid              = "PATH_INVALID"
	TextCodeOverrideTypeInvalid      = "OVERRIDE_TYPE_INVALID"
	TextCodePreferencesStoreRequired = "PREFERENCES_STORE_REQUIRED"
	TextCodeScopeInvalid             = "SCOPE_INVALID"
	TextCodeScopeMetadataMissing     = "SCOPE_METADATA_MISSING"
	TextCodeScopeMetadataInvalid     = "SCOPE_METADATA_INVALID"
	TextCodeAdapterFailed            = "ADAPTER_FAILED"
	TextCodeStoreReadFailed          = "STORE_READ_FAILED"
	TextCodeStoreWriteFailed         = "STORE_WRITE_FAILED"
	TextCodeDefaultLookupFailed      = "DEFAULT_LOOKUP_FAILED"
	TextCodeScopeResolveFailed       = "SCOPE_RESOLVE_FAILED"
)

var (
	ErrInvalidKey               = newSentinel(goerrors.CategoryBadInput, goerrors.CodeBadRequest, TextCodeInvalidKey, "feature key required")
	ErrStoreUnavailable         = newSentinel(goerrors.CategoryOperation, goerrors.CodeInternal, TextCodeStoreUnavailable, "override store not configured")
	ErrStoreRequired            = newSentinel(goerrors.CategoryOperation, goerrors.CodeInternal, TextCodeStoreRequired, "store is required")
	ErrResolverRequired         = newSentinel(goerrors.CategoryOperation, goerrors.CodeInternal, TextCodeResolverRequired, "resolver is required")
	ErrGateRequired             = newSentinel(goerrors.CategoryOperation, goerrors.CodeInternal, TextCodeGateRequired, "feature gate is required")
	ErrScopeRequired            = newSentinel(goerrors.CategoryBadInput, goerrors.CodeBadRequest, TextCodeScopeRequired, "scope is required")
	ErrSnapshotRequired         = newSentinel(goerrors.CategoryInternal, goerrors.CodeInternal, TextCodeSnapshotRequired, "snapshot is required")
	ErrPathRequired             = newSentinel(goerrors.CategoryBadInput, goerrors.CodeBadRequest, TextCodePathRequired, "path is required")
	ErrPathInvalid              = newSentinel(goerrors.CategoryBadInput, goerrors.CodeBadRequest, TextCodePathInvalid, "path segment is not a map")
	ErrPreferencesStoreRequired = newSentinel(goerrors.CategoryOperation, goerrors.CodeInternal, TextCodePreferencesStoreRequired, "preferences store is required")
)

func newSentinel(category goerrors.Category, code int, textCode, message string) *goerrors.Error {
	err := goerrors.New(message, category).WithTextCode(textCode)
	if code != 0 {
		err.WithCode(code)
	}
	return err
}

func IsSentinel(err error) bool {
	return err == ErrInvalidKey ||
		err == ErrStoreUnavailable ||
		err == ErrStoreRequired ||
		err == ErrResolverRequired ||
		err == ErrGateRequired ||
		err == ErrScopeRequired ||
		err == ErrSnapshotRequired ||
		err == ErrPathRequired ||
		err == ErrPathInvalid ||
		err == ErrPreferencesStoreRequired
}

func WrapSentinel(sentinel *goerrors.Error, message string, meta map[string]any) *goerrors.Error {
	if sentinel == nil {
		return nil
	}
	if message == "" {
		message = sentinel.Message
	}
	err := goerrors.New(message, sentinel.Category).
		WithTextCode(sentinel.TextCode).
		WithCode(sentinel.Code).
		WithSeverity(sentinel.Severity)
	err.Source = sentinel
	if meta != nil {
		err.WithMetadata(meta)
	}
	return err
}

func Wrap(err error, category goerrors.Category, textCode, message string, meta map[string]any) *goerrors.Error {
	if err == nil {
		return nil
	}
	if IsSentinel(err) {
		if sentinel, ok := err.(*goerrors.Error); ok {
			return WrapSentinel(sentinel, "", meta)
		}
	}
	if rich, ok := err.(*goerrors.Error); ok {
		clone := rich.Clone()
		if clone.TextCode == "" && textCode != "" {
			clone.TextCode = textCode
		}
		if clone.Message == "" && message != "" {
			clone.Message = message
		}
		if meta != nil {
			clone.WithMetadata(meta)
		}
		return clone
	}
	if message == "" {
		message = err.Error()
	}
	wrapped := goerrors.New(message, category).WithTextCode(textCode)
	wrapped.Source = err
	if meta != nil {
		wrapped.WithMetadata(meta)
	}
	return wrapped
}

func New(category goerrors.Category, textCode, message string, meta map[string]any) *goerrors.Error {
	err := goerrors.New(message, category).WithTextCode(textCode)
	if meta != nil {
		err.WithMetadata(meta)
	}
	return err
}

func NewBadInput(textCode, message string, meta map[string]any) *goerrors.Error {
	return New(goerrors.CategoryBadInput, textCode, message, meta)
}

func WrapBadInput(err error, textCode, message string, meta map[string]any) *goerrors.Error {
	return Wrap(err, goerrors.CategoryBadInput, textCode, message, meta)
}

func NewOperation(textCode, message string, meta map[string]any) *goerrors.Error {
	return New(goerrors.CategoryOperation, textCode, message, meta)
}

func WrapOperation(err error, textCode, message string, meta map[string]any) *goerrors.Error {
	return Wrap(err, goerrors.CategoryOperation, textCode, message, meta)
}

func NewExternal(textCode, message string, meta map[string]any) *goerrors.Error {
	return New(goerrors.CategoryExternal, textCode, message, meta)
}

func WrapExternal(err error, textCode, message string, meta map[string]any) *goerrors.Error {
	return Wrap(err, goerrors.CategoryExternal, textCode, message, meta)
}

func NewInternal(textCode, message string, meta map[string]any) *goerrors.Error {
	return New(goerrors.CategoryInternal, textCode, message, meta)
}

func WrapInternal(err error, textCode, message string, meta map[string]any) *goerrors.Error {
	return Wrap(err, goerrors.CategoryInternal, textCode, message, meta)
}

func As(err error) (*goerrors.Error, bool) {
	var rich *goerrors.Error
	if goerrors.As(err, &rich) {
		return rich, true
	}
	return nil, false
}
