package api

import (
	"net/http"
	"strings"

	"homelab/backend/internal/events"
	"homelab/backend/internal/notify"
	"homelab/backend/internal/store"
)

// handleRules lists rules (GET) or creates one (POST).
func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rules, err := s.state.ListRules(r.Context())
		if err != nil {
			s.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, rules)
	case http.MethodPost:
		var rule store.Rule
		if err := decodeJSON(r, &rule); err != nil {
			s.writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if rule.Metric == "" || rule.Operator == "" {
			s.writeErr(w, http.StatusBadRequest, "metric and operator are required")
			return
		}
		rule.Enabled = true
		if err := s.state.UpsertRule(r.Context(), &rule); err != nil {
			s.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.ruleEng.Reload(r.Context())
		s.writeJSON(w, http.StatusOK, rule)
	default:
		s.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleRuleDelete removes a rule (DELETE /api/v1/rules/:id).
func (s *Server) handleRuleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/rules/")
	if id == "" {
		s.writeErr(w, http.StatusBadRequest, "missing rule id")
		return
	}
	if err := s.state.DeleteRule(r.Context(), id); err != nil {
		s.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.ruleEng.Reload(r.Context())
	s.writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}

// handleSettings lists settings and provider status.
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	settings, err := s.state.ListSettings(r.Context())
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	providers := []map[string]any{}
	for _, p := range s.notif.Registry().All() {
		providers = append(providers, map[string]any{
			"name":       p.Name(),
			"configured": p.Configured(),
		})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"settings":  settings,
		"providers": providers,
		"config": map[string]any{
			"collect_interval": s.cfg.CollectInterval.String(),
			"retention":        s.cfg.Retention.String(),
			"demo":             s.cfg.Demo,
		},
	})
}

// handleSettingsPut updates a single setting (PUT /api/v1/settings/:key).
func (s *Server) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		s.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/api/v1/settings/")
	if key == "" {
		s.writeErr(w, http.StatusBadRequest, "missing key")
		return
	}
	var body struct {
		Value string `json:"value"`
	}
	if err := decodeJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.state.SetSetting(r.Context(), key, body.Value); err != nil {
		s.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": body.Value})
}

// handleNotifyProviders lists notification providers and their configuration
// status.
func (s *Server) handleNotifyProviders(w http.ResponseWriter, r *http.Request) {
	out := []map[string]any{}
	for _, p := range s.notif.Registry().All() {
		out = append(out, map[string]any{
			"name":       p.Name(),
			"configured": p.Configured(),
		})
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleNotifyConfig returns the effective (masked) notification config.
func (s *Server) handleNotifyConfig(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, s.notif.ViewFor())
}

// handleNotifyConfigPut applies a partial config update and reloads providers.
func (s *Server) handleNotifyConfigPut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		s.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body notify.UpdateRequest
	if err := decodeJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.notif.Update(r.Context(), body); err != nil {
		s.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	ev := events.New(events.TypeNotificationSent, events.SeverityInfo, "settings",
		"Notification configuration updated")
	s.tee.Emit(r.Context(), ev)
	s.writeJSON(w, http.StatusOK, s.notif.ViewFor())
}

// handleNotifyTest sends a test notification via every configured provider.
func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ctx := r.Context()
	msg := notify.Message{
		Title:    "Node",
		Message:  "Test notification from this node",
		Severity: "info",
		Tags:     []string{"white_check_mark"},
	}
	sent := 0
	for _, p := range s.notif.Registry().All() {
		if !p.Configured() {
			continue
		}
		if err := p.Send(ctx, msg); err != nil {
			s.log.Warn("test notification failed", "provider", p.Name(), "error", err)
			continue
		}
		sent++
	}
	if sent == 0 {
		s.writeErr(w, http.StatusBadRequest, "no notification channel is configured")
		return
	}
	ev := events.New(events.TypeNotificationSent, events.SeverityInfo, "settings",
		"Test notification sent")
	ev.Data = map[string]any{"channels": sent}
	s.tee.Emit(ctx, ev)
	s.writeJSON(w, http.StatusOK, map[string]any{"status": "sent", "channels": sent})
}
