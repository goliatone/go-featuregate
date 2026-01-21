package ferrors

import (
	"errors"
	"testing"

	goerrors "github.com/goliatone/go-errors"
)

func TestWrapSentinelPreservesIsAndMetadata(t *testing.T) {
	err := WrapSentinel(ErrInvalidKey, "", map[string]any{
		MetaFeatureKey: "users.signup",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("expected errors.Is to match sentinel")
	}
	rich, ok := As(err)
	if !ok {
		t.Fatalf("expected rich error")
	}
	if rich.Category != goerrors.CategoryBadInput {
		t.Fatalf("unexpected category: %s", rich.Category)
	}
	if rich.TextCode != TextCodeInvalidKey {
		t.Fatalf("unexpected text code: %s", rich.TextCode)
	}
	if rich.Metadata == nil || rich.Metadata[MetaFeatureKey] != "users.signup" {
		t.Fatalf("expected metadata to include feature key")
	}
}
