package notify

import (
	"context"
	"testing"
)

// TestManagerReloadLayers verifies persisted settings are layered over the
// env/flag defaults when reloading.
func TestManagerReloadLayers(t *testing.T) {
	ctx := context.Background()
	store := map[string]string{
		KeyNtfyTopic:    "alerts",
		KeySmtpHost:     "smtp.example.com",
		KeySmtpInsecure: "true",
		KeySmtpPort:     "465",
		KeySmtpFrom:     "homelab@example.com",
		KeySmtpTo:       "me@example.com",
	}
	base := Config{NtfyServer: "https://ntfy.sh", NtfyTopic: "default", SmtpHost: "smtp.default", SmtpPort: 587}
	m := NewManager(base, func(ctx context.Context) (map[string]string, error) {
		return store, nil
	}, func(ctx context.Context, k, v string) error {
		store[k] = v
		return nil
	})

	if err := m.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	cfg := m.EffectiveConfig()
	if cfg.NtfyServer != "https://ntfy.sh" || cfg.NtfyTopic != "alerts" {
		t.Fatalf("layering wrong: %+v", cfg)
	}
	if cfg.SmtpHost != "smtp.example.com" || cfg.SmtpPort != 465 || !cfg.SmtpInsecure {
		t.Fatalf("smtp layering wrong: %+v", cfg)
	}
	if p, ok := m.Registry().Get("ntfy"); !ok || !p.Configured() {
		t.Fatal("ntfy provider should be configured")
	}
	if p, ok := m.Registry().Get("email"); !ok || !p.Configured() {
		t.Fatal("email provider should be configured after reload")
	}
}

// TestManagerUpdate verifies partial updates persist and reload providers, and
// that an empty password leaves the stored one untouched.
func TestManagerUpdate(t *testing.T) {
	ctx := context.Background()
	store := map[string]string{}
	base := Config{SmtpHost: "smtp.default", SmtpPort: 587, SmtpPass: "secret"}
	m := NewManager(base, func(ctx context.Context) (map[string]string, error) {
		return store, nil
	}, func(ctx context.Context, k, v string) error {
		store[k] = v
		return nil
	})

	host := "smtp.updated"
	port := 465
	empty := ""
	if err := m.Update(ctx, UpdateRequest{SmtpHost: &host, SmtpPort: &port, SmtpPass: &empty}); err != nil {
		t.Fatal(err)
	}
	cfg := m.EffectiveConfig()
	if cfg.SmtpHost != "smtp.updated" || cfg.SmtpPort != 465 {
		t.Fatalf("update not applied: %+v", cfg)
	}
	if cfg.SmtpPass != "secret" {
		t.Fatalf("empty password should keep stored secret, got %q", cfg.SmtpPass)
	}
	if store[KeySmtpPass] != "" {
		t.Fatal("empty password should not be persisted")
	}
	if store[KeySmtpHost] != "smtp.updated" {
		t.Fatal("host should be persisted")
	}
}

// TestSplitList verifies recipient list parsing tolerates mixed separators.
func TestSplitList(t *testing.T) {
	got := splitList("a@x.com, b@y.com;c@z.com")
	if len(got) != 3 || got[0] != "a@x.com" || got[1] != "b@y.com" || got[2] != "c@z.com" {
		t.Fatalf("unexpected split: %#v", got)
	}
}
