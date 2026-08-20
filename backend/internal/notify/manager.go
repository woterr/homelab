package notify

import (
	"context"
	"strconv"
	"strings"
	"sync"
)

// Config is the effective configuration for all notification providers. It is
// seeded from flags/env and overlaid with persisted settings so the channels
// can be managed from the dashboard without restarting the daemon.
type Config struct {
	NtfyServer     string
	NtfyTopic      string
	DiscordWebhook string
	SmtpHost       string
	SmtpPort       int
	SmtpUser       string
	SmtpPass       string
	SmtpFrom       string
	SmtpTo         string
	SmtpInsecure   bool
	PhoneWebhook   string
	PhoneTo        string
}

// View is the masked representation returned to the UI (passwords are never
// echoed back; PassSet reports whether one is stored).
type View struct {
	NtfyServer     string `json:"ntfy_server"`
	NtfyTopic      string `json:"ntfy_topic"`
	DiscordWebhook string `json:"discord_webhook"`
	SmtpHost       string `json:"smtp_host"`
	SmtpPort       int    `json:"smtp_port"`
	SmtpUser       string `json:"smtp_user"`
	SmtpFrom       string `json:"smtp_from"`
	SmtpTo         string `json:"smtp_to"`
	SmtpInsecure   bool   `json:"smtp_insecure"`
	PassSet        bool   `json:"smtp_pass_set"`
	PhoneWebhook   string `json:"phone_webhook"`
	PhoneTo        string `json:"phone_to"`
}

// UpdateRequest carries partial updates from the UI. Omitted (nil) fields are
// left unchanged; an empty string clears a field; SmtpPass is only persisted
// when non-empty.
type UpdateRequest struct {
	NtfyServer     *string `json:"ntfy_server,omitempty"`
	NtfyTopic      *string `json:"ntfy_topic,omitempty"`
	DiscordWebhook *string `json:"discord_webhook,omitempty"`
	SmtpHost       *string `json:"smtp_host,omitempty"`
	SmtpPort       *int    `json:"smtp_port,omitempty"`
	SmtpUser       *string `json:"smtp_user,omitempty"`
	SmtpPass       *string `json:"smtp_pass,omitempty"`
	SmtpFrom       *string `json:"smtp_from,omitempty"`
	SmtpTo         *string `json:"smtp_to,omitempty"`
	SmtpInsecure   *bool   `json:"smtp_insecure,omitempty"`
	PhoneWebhook   *string `json:"phone_webhook,omitempty"`
	PhoneTo        *string `json:"phone_to,omitempty"`
}

// Settings keys persisted via the StateStore.
const (
	KeyNtfyServer   = "notify.ntfy.server"
	KeyNtfyTopic    = "notify.ntfy.topic"
	KeyDiscordHook  = "notify.discord.webhook"
	KeySmtpHost     = "notify.smtp.host"
	KeySmtpPort     = "notify.smtp.port"
	KeySmtpUser     = "notify.smtp.user"
	KeySmtpPass     = "notify.smtp.pass"
	KeySmtpFrom     = "notify.smtp.from"
	KeySmtpTo       = "notify.smtp.to"
	KeySmtpInsecure = "notify.smtp.insecure"
	KeyPhoneHook    = "notify.phone.webhook"
	KeyPhoneTo      = "notify.phone.to"
)

// Manager owns the provider registry and reconciles env-defaulted config with
// persisted settings. Changes are applied live without restarting.
type Manager struct {
	reg  *Registry
	base Config
	keys func(ctx context.Context) (map[string]string, error)
	save func(ctx context.Context, key, value string) error
	mu   sync.Mutex
	cfg  Config
}

// NewManager builds a manager seeded with the env/flag defaults. keys must
// return persisted settings (state.ListSettings); save persists one key
// (state.SetSetting).
func NewManager(base Config, keys func(ctx context.Context) (map[string]string, error),
	save func(ctx context.Context, key, value string) error) *Manager {
	m := &Manager{reg: NewRegistry(), base: base, keys: keys, save: save, cfg: base}
	m.reg.ReplaceAll(buildProviders(base))
	return m
}

func (m *Manager) Registry() *Registry { return m.reg }

// Reload re-reads persisted settings and rebuilds providers.
func (m *Manager) Reload(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kv, err := m.keys(ctx)
	if err != nil {
		return err
	}
	m.cfg = merge(m.base, kv)
	m.reg.ReplaceAll(buildProviders(m.cfg))
	return nil
}

// Update applies a partial config, persists the changed keys and rebuilds
// providers. Empty SmtpPass keeps the stored value.
func (m *Manager) Update(ctx context.Context, u UpdateRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.cfg
	apply := func(key, val string) {
		if err := m.save(ctx, key, val); err != nil {
			return
		}
		next = merge(next, map[string]string{key: val})
	}
	if u.NtfyServer != nil {
		apply(KeyNtfyServer, *u.NtfyServer)
	}
	if u.NtfyTopic != nil {
		apply(KeyNtfyTopic, *u.NtfyTopic)
	}
	if u.DiscordWebhook != nil {
		apply(KeyDiscordHook, *u.DiscordWebhook)
	}
	if u.SmtpHost != nil {
		apply(KeySmtpHost, *u.SmtpHost)
	}
	if u.SmtpPort != nil {
		apply(KeySmtpPort, strconv.Itoa(*u.SmtpPort))
	}
	if u.SmtpUser != nil {
		apply(KeySmtpUser, *u.SmtpUser)
	}
	if u.SmtpPass != nil && *u.SmtpPass != "" {
		apply(KeySmtpPass, *u.SmtpPass)
	}
	if u.SmtpFrom != nil {
		apply(KeySmtpFrom, *u.SmtpFrom)
	}
	if u.SmtpTo != nil {
		apply(KeySmtpTo, *u.SmtpTo)
	}
	if u.SmtpInsecure != nil {
		apply(KeySmtpInsecure, strconv.FormatBool(*u.SmtpInsecure))
	}
	if u.PhoneWebhook != nil {
		apply(KeyPhoneHook, *u.PhoneWebhook)
	}
	if u.PhoneTo != nil {
		apply(KeyPhoneTo, *u.PhoneTo)
	}
	m.cfg = next
	m.reg.ReplaceAll(buildProviders(m.cfg))
	return nil
}

// EffectiveConfig returns the current configuration.
func (m *Manager) EffectiveConfig() Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// ViewFor returns the masked config for the UI.
func (m *Manager) ViewFor() View {
	cfg := m.EffectiveConfig()
	return View{
		NtfyServer:     cfg.NtfyServer,
		NtfyTopic:      cfg.NtfyTopic,
		DiscordWebhook: cfg.DiscordWebhook,
		SmtpHost:       cfg.SmtpHost,
		SmtpPort:       cfg.SmtpPort,
		SmtpUser:       cfg.SmtpUser,
		SmtpFrom:       cfg.SmtpFrom,
		SmtpTo:         cfg.SmtpTo,
		SmtpInsecure:   cfg.SmtpInsecure,
		PassSet:        cfg.SmtpPass != "",
		PhoneWebhook:   cfg.PhoneWebhook,
		PhoneTo:        cfg.PhoneTo,
	}
}

// merge layers persisted settings over the default config.
func merge(base Config, kv map[string]string) Config {
	cfg := base
	if v, ok := kv[KeyNtfyServer]; ok {
		cfg.NtfyServer = v
	}
	if v, ok := kv[KeyNtfyTopic]; ok {
		cfg.NtfyTopic = v
	}
	if v, ok := kv[KeyDiscordHook]; ok {
		cfg.DiscordWebhook = v
	}
	if v, ok := kv[KeySmtpHost]; ok {
		cfg.SmtpHost = v
	}
	if v, ok := kv[KeySmtpPort]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.SmtpPort = n
		}
	}
	if v, ok := kv[KeySmtpUser]; ok {
		cfg.SmtpUser = v
	}
	if v, ok := kv[KeySmtpPass]; ok {
		cfg.SmtpPass = v
	}
	if v, ok := kv[KeySmtpFrom]; ok {
		cfg.SmtpFrom = v
	}
	if v, ok := kv[KeySmtpTo]; ok {
		cfg.SmtpTo = v
	}
	if v, ok := kv[KeySmtpInsecure]; ok {
		cfg.SmtpInsecure = v == "true" || v == "1"
	}
	if v, ok := kv[KeyPhoneHook]; ok {
		cfg.PhoneWebhook = v
	}
	if v, ok := kv[KeyPhoneTo]; ok {
		cfg.PhoneTo = v
	}
	return cfg
}

// buildProviders constructs the full provider set from a config.
func buildProviders(cfg Config) []Provider {
	return []Provider{
		NewNtfy(NtfyConfig{Server: cfg.NtfyServer, Topic: cfg.NtfyTopic}),
		NewDiscord(DiscordConfig{WebhookURL: cfg.DiscordWebhook}),
		NewEmail(EmailConfig{
			Host:     cfg.SmtpHost,
			Port:     cfg.SmtpPort,
			User:     cfg.SmtpUser,
			Pass:     cfg.SmtpPass,
			From:     cfg.SmtpFrom,
			To:       splitList(cfg.SmtpTo),
			Insecure: cfg.SmtpInsecure,
		}),
		NewPhone(PhoneConfig{WebhookURL: cfg.PhoneWebhook, To: cfg.PhoneTo}),
		NewStub("telegram"),
	}
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
