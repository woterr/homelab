package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRuleChannelsRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, err := OpenStateStore(ctx, filepath.Join(t.TempDir(), "rules.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := &Rule{Name: "x", Metric: "temperature{a}", Operator: "lt", Threshold: 80, Notify: true, Channels: []string{"discord", "ntfy"}}
	if err := s.UpsertRule(ctx, r); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListRules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d rules", len(list))
	}
	if len(list[0].Channels) != 2 || list[0].Channels[0] != "discord" {
		t.Fatalf("channels = %v", list[0].Channels)
	}
}
