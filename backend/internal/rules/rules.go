// Package rules implements the V1 threshold engine. Rules watch a metric
// against an operator/threshold, optionally sustained for a duration, and fire
// an event plus an optional notification. It is the seed of the future visual
// workflow system.
package rules

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"homelab/backend/internal/events"
	"homelab/backend/internal/log"
	"homelab/backend/internal/metric"
	"homelab/backend/internal/notify"
	"homelab/backend/internal/store"
)

// Engine evaluates persisted rules against collected samples each tick.
type Engine struct {
	store *store.StateStore
	tee   *events.Tee
	notif *notify.Registry
	log   *log.Logger

	mu     sync.Mutex
	rules  map[string]*store.Rule
	since  map[string]time.Time // when a rule's condition first held
	firing map[string]bool
}

func NewEngine(st *store.StateStore, tee *events.Tee, n *notify.Registry, lg *log.Logger) *Engine {
	e := &Engine{
		store:  st,
		tee:    tee,
		notif:  n,
		log:    lg,
		rules:  map[string]*store.Rule{},
		since:  map[string]time.Time{},
		firing: map[string]bool{},
	}
	e.Reload(context.Background())
	return e
}

// Reload re-reads rules from the state store.
func (e *Engine) Reload(ctx context.Context) {
	list, err := e.store.ListRules(ctx)
	if err != nil {
		e.log.Warn("rule reload failed", "error", err)
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = map[string]*store.Rule{}
	for _, r := range list {
		e.rules[r.ID] = r
	}
}

// Evaluate runs all enabled rules against a batch of samples (one collector
// tick).
func (e *Engine) Evaluate(ctx context.Context, samples []metric.Sample) {
	now := time.Now()
	bySeries := map[string][]metric.Sample{}
	for _, s := range samples {
		bySeries[s.Series] = append(bySeries[s.Series], s)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for id, r := range e.rules {
		if !r.Enabled {
			continue
		}
		vals := bySeries[r.Metric]
		if len(vals) == 0 {
			continue
		}
		// A labeled series matches only when the series stem matches exactly;
		// use the latest sample value of the first match.
		v := vals[len(vals)-1].Value
		met := compare(v, r.Operator, r.Threshold)

		since, hasSince := e.since[id]
		if met {
			if !hasSince {
				e.since[id] = now
			}
			elapsed := now.Sub(since)
			need := time.Duration(r.DurationS) * time.Second
			if !e.firing[id] && elapsed >= need {
				e.firing[id] = true
				e.fire(ctx, r, v)
			}
			continue
		}
		// Condition no longer holds.
		if e.firing[id] {
			e.firing[id] = false
			e.recover(ctx, r, v)
		}
		delete(e.since, id)
	}
}

func compare(v float64, op string, threshold float64) bool {
	switch strings.ToLower(op) {
	case "gt", ">":
		return v > threshold
	case "gte", ">=":
		return v >= threshold
	case "lt", "<":
		return v < threshold
	case "lte", "<=":
		return v <= threshold
	case "eq", "==":
		return v == threshold
	case "neq", "!=":
		return v != threshold
	}
	return false
}

func (e *Engine) fire(ctx context.Context, r *store.Rule, v float64) {
	msg := r.Message
	if msg == "" {
		msg = ruleMessage(r, v)
	}
	ev := events.New(events.TypeThresholdExceeded, events.SeverityWarning, "rules", msg)
	ev.Data = map[string]any{"rule": r.Name, "metric": r.Metric, "value": v, "threshold": r.Threshold}
	e.tee.Emit(ctx, ev)

	if r.Notify {
		e.notify(ctx, r, v, false)
	}
}

func (e *Engine) recover(ctx context.Context, r *store.Rule, v float64) {
	ev := events.New(events.TypeThresholdRecovered, events.SeverityInfo, "rules",
		r.Name+" recovered ("+r.Metric+" = "+trimNum(v)+")")
	ev.Data = map[string]any{"rule": r.Name, "metric": r.Metric, "value": v}
	e.tee.Emit(ctx, ev)
}

func (e *Engine) notify(ctx context.Context, r *store.Rule, v float64, recovered bool) {
	title := "Node: " + r.Name
	if recovered {
		title = "Node: " + r.Name + " recovered"
	}
	msg := ruleMessage(r, v)
	m := notify.Message{Title: title, Message: msg, Severity: "warning"}
	sent := 0
	for _, p := range e.notif.All() {
		if !p.Configured() {
			continue
		}
		// A rule may target specific channels ("discord", "ntfy", ...); when it
		// lists none, notify every configured provider.
		if len(r.Channels) > 0 && !contains(r.Channels, p.Name()) {
			continue
		}
		if err := p.Send(ctx, m); err != nil {
			e.log.Warn("notification send failed", "provider", p.Name(), "error", err)
			continue
		}
		sent++
	}
	if sent > 0 {
		nev := events.New(events.TypeNotificationSent, events.SeverityInfo, "rules",
			"Notification sent for "+r.Name)
		nev.Data = map[string]any{"channels": sent, "rule": r.Name}
		e.tee.Emit(ctx, nev)
	}
}

func ruleMessage(r *store.Rule, v float64) string {
	return r.Name + ": " + r.Metric + " " + trimNum(v) + " " + r.Operator + " " + trimNum(r.Threshold)
}

func trimNum(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}
