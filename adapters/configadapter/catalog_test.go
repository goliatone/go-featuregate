package configadapter

import "testing"

func TestCatalogFromNestedMap(t *testing.T) {
	cat := NewCatalog(map[string]any{
		"users": map[string]any{
			"signup": map[string]any{
				"description": "Allow signups",
			},
			"invite": "Allow invites",
			"password_reset": map[string]any{
				"description": map[string]any{
					"key":  "feature.users.password_reset.description",
					"text": "Allow password reset",
					"args": map[string]any{"window": "30d"},
				},
			},
		},
	})

	def, ok := cat.Get("users.signup")
	if !ok {
		t.Fatalf("expected users.signup to exist")
	}
	if def.Description.Text != "Allow signups" {
		t.Fatalf("unexpected description: %q", def.Description.Text)
	}

	def, ok = cat.Get("users.invite")
	if !ok {
		t.Fatalf("expected users.invite to exist")
	}
	if def.Description.Text != "Allow invites" {
		t.Fatalf("unexpected description: %q", def.Description.Text)
	}

	def, ok = cat.Get("users.password_reset")
	if !ok {
		t.Fatalf("expected users.password_reset to exist")
	}
	if def.Description.Key != "feature.users.password_reset.description" {
		t.Fatalf("unexpected description key: %q", def.Description.Key)
	}
	if def.Description.Text != "Allow password reset" {
		t.Fatalf("unexpected description text: %q", def.Description.Text)
	}
	if def.Description.Args == nil || def.Description.Args["window"] != "30d" {
		t.Fatalf("expected args to be set")
	}
}

func TestCatalogDescriptionKeyFields(t *testing.T) {
	cat := NewCatalog(map[string]any{
		"users.signup": map[string]any{
			"description_key":  "feature.users.signup.description",
			"description_text": "Allow signups",
		},
	})

	def, ok := cat.Get("users.signup")
	if !ok {
		t.Fatalf("expected users.signup to exist")
	}
	if def.Description.Key != "feature.users.signup.description" {
		t.Fatalf("unexpected description key: %q", def.Description.Key)
	}
	if def.Description.Text != "Allow signups" {
		t.Fatalf("unexpected description text: %q", def.Description.Text)
	}
}
