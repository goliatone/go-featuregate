package catalog

import (
	"context"
	"testing"
)

func TestStaticCatalogGetNormalizesKey(t *testing.T) {
	cat := NewStatic(map[string]FeatureDefinition{
		"users.signup": {
			Key: "users.signup",
			Description: Message{
				Text: "Allow signups",
			},
		},
	})

	def, ok := cat.Get(" users.signup ")
	if !ok {
		t.Fatalf("expected definition to be found")
	}
	if def.Key != "users.signup" {
		t.Fatalf("expected normalized key, got %q", def.Key)
	}
	if def.Description.Text != "Allow signups" {
		t.Fatalf("unexpected description: %q", def.Description.Text)
	}
}

func TestPlainResolverPrefersText(t *testing.T) {
	resolver := PlainResolver{}
	msg := Message{
		Key:  "feature.users.signup.description",
		Text: "Allow signups",
	}
	value, err := resolver.Resolve(context.Background(), "en", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "Allow signups" {
		t.Fatalf("expected text to be returned, got %q", value)
	}

	msg = Message{
		Key: "feature.users.signup.description",
	}
	value, err = resolver.Resolve(context.Background(), "en", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "feature.users.signup.description" {
		t.Fatalf("expected key to be returned, got %q", value)
	}
}
