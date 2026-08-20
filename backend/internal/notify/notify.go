// Package notify defines the notification provider abstraction and the V1 ntfy
// implementation. Future providers (Telegram, email, web push) implement the
// same Provider interface and are registered here.
package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

// Message is a normalized notification payload.
type Message struct {
	Title    string   `json:"title"`
	Message  string   `json:"message"`
	Severity string   `json:"severity"` // info | warning | error | critical
	Tags     []string `json:"tags,omitempty"`
	URL      string   `json:"url,omitempty"`
}

// Provider sends notifications.
type Provider interface {
	// Name returns the provider identifier (e.g. "ntfy").
	Name() string
	// Configured reports whether the provider is ready to send.
	Configured() bool
	// Send delivers a message.
	Send(ctx context.Context, m Message) error
}

// Registry maps provider names to instances.
type Registry struct {
	providers map[string]Provider
	order     []string
}

func NewRegistry() *Registry {
	return &Registry{providers: map[string]Provider{}}
}

func (r *Registry) Register(p Provider) {
	r.providers[p.Name()] = p
	r.order = append(r.order, p.Name())
}

// ReplaceAll swaps the full provider set, preserving the registry instance so
// holders keep a stable reference (used when settings change at runtime).
func (r *Registry) ReplaceAll(providers []Provider) {
	r.providers = map[string]Provider{}
	r.order = nil
	for _, p := range providers {
		r.Register(p)
	}
}

func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

func (r *Registry) Names() []string { return r.order }

func (r *Registry) All() []Provider {
	out := make([]Provider, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.providers[n])
	}
	return out
}

// ---- ntfy provider ----

// NtfyConfig holds ntfy connection settings.
type NtfyConfig struct {
	Server string // base URL, e.g. https://ntfy.sh
	Topic  string // topic to publish to
}

// Ntfy sends notifications to an ntfy server (ntfy.sh or self-hosted).
type Ntfy struct {
	cfg NtfyConfig
	hc  *http.Client
}

func NewNtfy(cfg NtfyConfig) *Ntfy {
	return &Ntfy{
		cfg: cfg,
		hc:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *Ntfy) Name() string { return "ntfy" }

func (n *Ntfy) Configured() bool { return n.cfg.Server != "" && n.cfg.Topic != "" }

func (n *Ntfy) Send(ctx context.Context, m Message) error {
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", n.cfg.Server, n.cfg.Topic)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ---- Discord provider ----

// DiscordConfig holds Discord webhook settings.
type DiscordConfig struct {
	WebhookURL string
}

// Discord posts messages to a Discord channel via a webhook URL.
type Discord struct {
	url string
	hc  *http.Client
}

func NewDiscord(cfg DiscordConfig) *Discord {
	return &Discord{url: cfg.WebhookURL, hc: &http.Client{Timeout: 10 * time.Second}}
}

func (d *Discord) Name() string { return "discord" }

func (d *Discord) Configured() bool { return d.url != "" }

func (d *Discord) Send(ctx context.Context, m Message) error {
	body, err := json.Marshal(map[string]string{
		"username": "Node",
		"content":  m.Title + "\n" + m.Message,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("discord: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ---- Email (SMTP) provider ----

// EmailConfig holds SMTP connection settings.
type EmailConfig struct {
	Host     string
	Port     int
	User     string
	Pass     string
	From     string
	To       []string
	Insecure bool // allow TLS with a self-signed cert
}

// Email sends notifications via SMTP (with STARTTLS when available).
type Email struct {
	cfg EmailConfig
}

func NewEmail(cfg EmailConfig) *Email { return &Email{cfg: cfg} }

func (e *Email) Name() string { return "email" }

func (e *Email) Configured() bool {
	return e.cfg.Host != "" && e.cfg.Port > 0 && e.cfg.From != "" && len(e.cfg.To) > 0
}

func (e *Email) Send(ctx context.Context, m Message) error {
	addr := fmt.Sprintf("%s:%d", e.cfg.Host, e.cfg.Port)
	var auth smtp.Auth
	if e.cfg.User != "" {
		auth = smtp.PlainAuth("", e.cfg.User, e.cfg.Pass, e.cfg.Host)
	}
	msg := []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		e.cfg.From, strings.Join(e.cfg.To, ","), m.Title, m.Message,
	))

	done := make(chan error, 1)
	go func() {
		var err error
		if e.cfg.Insecure {
			err = e.sendTLS(addr, auth, e.cfg.From, e.cfg.To, msg)
		} else {
			err = smtp.SendMail(addr, auth, e.cfg.From, e.cfg.To, msg)
		}
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// sendTLS dials with implicit TLS (port 465 style).
func (e *Email) sendTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: e.cfg.Insecure})
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, e.cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// ---- Phone (SMS webhook) provider ----

// PhoneConfig holds settings for a generic SMS gateway webhook.
type PhoneConfig struct {
	WebhookURL string // receives {"to": phone, "text": message}
	To         string
}

// Phone sends SMS through a generic gateway webhook.
type Phone struct {
	url string
	to  string
	hc  *http.Client
}

func NewPhone(cfg PhoneConfig) *Phone {
	return &Phone{url: cfg.WebhookURL, to: cfg.To, hc: &http.Client{Timeout: 10 * time.Second}}
}

func (p *Phone) Name() string { return "phone" }

func (p *Phone) Configured() bool { return p.url != "" && p.to != "" }

func (p *Phone) Send(ctx context.Context, m Message) error {
	body, err := json.Marshal(map[string]string{
		"to":   p.to,
		"text": m.Title + "\n" + m.Message,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("phone: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ---- stubs for future providers ----

// Stub is a not-configured provider registered so the UI can list available
// notification channels before they are enabled.
type Stub struct {
	name string
}

// NewStub creates an unconfigured provider placeholder.
func NewStub(name string) *Stub { return &Stub{name: name} }

func (s *Stub) Name() string     { return s.name }
func (s *Stub) Configured() bool { return false }
func (s *Stub) Send(context.Context, Message) error {
	return fmt.Errorf("provider %s is not configured", s.name)
}
