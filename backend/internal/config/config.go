// Package config defines runtime configuration for the homelab daemon.
package config

import (
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Defaults for a lightweight single-node deployment.
const (
	DefaultListenAddr = ":8000"
	DefaultCollectInt = 5 * time.Second
	DefaultRetention  = 30 * 24 * time.Hour
	DefaultRollupWin  = 48 * time.Hour
	DefaultRollupInt  = 15 * time.Minute
	DefaultNtfyServer = "https://ntfy.sh"
	DefaultSweepEvery = 90 * time.Second
	DefaultSweepRetry = 30 * time.Second
	DefaultHistory    = 15 * time.Minute
)

type Config struct {
	Demo       bool
	DataDir    string
	ListenAddr string
	HostProc   string
	HostSys    string
	HostDocker string
	HostRoot   string
	LogLevel   slog.Level

	CollectInterval time.Duration
	Retention       time.Duration
	RollupWindow    time.Duration
	RollupInterval  time.Duration

	NtfyServer string
	NtfyTopic  string

	DiscordWebhook string

	SmtpHost     string
	SmtpPort     int
	SmtpUser     string
	SmtpPass     string
	SmtpFrom     string
	SmtpTo       string
	SmtpInsecure bool

	PhoneWebhook string
	PhoneTo      string

	DeviceSweepEnabled bool
	DeviceSweepEvery   time.Duration
	DeviceSweepRetry   time.Duration

	HistoryWindow time.Duration
}

func New() *Config {
	return &Config{
		DataDir:            "./data",
		ListenAddr:         DefaultListenAddr,
		HostProc:           "/proc",
		HostSys:            "/sys",
		HostDocker:         "/var/run/docker.sock",
		LogLevel:           slog.LevelInfo,
		CollectInterval:    DefaultCollectInt,
		Retention:          DefaultRetention,
		RollupWindow:       DefaultRollupWin,
		RollupInterval:     DefaultRollupInt,
		NtfyServer:         DefaultNtfyServer,
		DeviceSweepEnabled: true,
		DeviceSweepEvery:   DefaultSweepEvery,
		DeviceSweepRetry:   DefaultSweepRetry,
		HistoryWindow:      DefaultHistory,
	}
}

func (c *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.Demo, "demo", c.Demo, "run collectors against a deterministic simulated host")
	fs.StringVar(&c.DataDir, "data-dir", c.DataDir, "directory for the SQLite database")
	fs.StringVar(&c.ListenAddr, "listen", c.ListenAddr, "HTTP listen address")
	fs.StringVar(&c.HostProc, "host-proc", c.HostProc, "path to the host /proc (for containers)")
	fs.StringVar(&c.HostSys, "host-sys", c.HostSys, "path to the host /sys (for containers)")
	fs.StringVar(&c.HostDocker, "host-docker", c.HostDocker, "path to the Docker socket")
	fs.StringVar(&c.HostRoot, "host-root", c.HostRoot, "path to a read-only bind of the host root (for containers)")
	fs.DurationVar(&c.CollectInterval, "collect-interval", c.CollectInterval, "collector tick interval")
	fs.DurationVar(&c.Retention, "retention", c.Retention, "metric retention window")
	fs.DurationVar(&c.RollupWindow, "rollup-window", c.RollupWindow, "age after which raw samples are folded into rollups")
	fs.DurationVar(&c.RollupInterval, "rollup-interval", c.RollupInterval, "how often the rollup/retention janitor runs")
	fs.StringVar(&c.NtfyServer, "ntfy-server", c.NtfyServer, "ntfy server base URL")
	fs.StringVar(&c.NtfyTopic, "ntfy-topic", "", "ntfy topic; notifications enabled when set")
	fs.StringVar(&c.DiscordWebhook, "discord-webhook", "", "Discord webhook URL; notifications enabled when set")
	fs.StringVar(&c.SmtpHost, "smtp-host", "", "SMTP host for email notifications")
	fs.IntVar(&c.SmtpPort, "smtp-port", c.SmtpPort, "SMTP port (587 STARTTLS, 465 implicit TLS)")
	fs.StringVar(&c.SmtpUser, "smtp-user", "", "SMTP username (plain auth)")
	fs.StringVar(&c.SmtpPass, "smtp-pass", "", "SMTP password")
	fs.StringVar(&c.SmtpFrom, "smtp-from", "", "email From address")
	fs.StringVar(&c.SmtpTo, "smtp-to", "", "comma-separated recipient addresses")
	fs.BoolVar(&c.SmtpInsecure, "smtp-insecure", false, "allow TLS with self-signed SMTP certs")
	fs.StringVar(&c.PhoneWebhook, "phone-webhook", "", "SMS gateway webhook URL; receives JSON {to,text}")
	fs.StringVar(&c.PhoneTo, "phone-to", "", "destination phone number for SMS notifications")
	fs.BoolVar(&c.DeviceSweepEnabled, "device-sweep", c.DeviceSweepEnabled, "enable the gentle LAN device sweep")
	fs.DurationVar(&c.DeviceSweepEvery, "device-sweep-every", c.DeviceSweepEvery, "interval between LAN sweeps")
	fs.DurationVar(&c.DeviceSweepRetry, "device-sweep-retry", c.DeviceSweepRetry, "probe timeout / retry backoff for a sweep")
	fs.DurationVar(&c.HistoryWindow, "history-window", c.HistoryWindow, "default live history window for metric streams")
}

// DBPath resolves the sqlite database path.
func (c *Config) DBPath() string { return filepath.Join(c.DataDir, "homelab.db") }

// Env overrides known settings from environment variables (used by docker-compose).
func (c *Config) Env() {
	if v := os.Getenv("DATA_DIR"); v != "" {
		c.DataDir = v
	}
	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		c.ListenAddr = v
	}
	if v := os.Getenv("HOST_PROC"); v != "" {
		c.HostProc = v
	}
	if v := os.Getenv("HOST_SYS"); v != "" {
		c.HostSys = v
	}
	if v := os.Getenv("HOST_DOCKER"); v != "" {
		c.HostDocker = v
	}
	if v := os.Getenv("HOST_ROOT"); v != "" {
		c.HostRoot = v
	}
	if v := os.Getenv("NTFY_SERVER"); v != "" {
		c.NtfyServer = v
	}
	if v := os.Getenv("NTFY_TOPIC"); v != "" {
		c.NtfyTopic = v
	}
	if v := os.Getenv("DISCORD_WEBHOOK"); v != "" {
		c.DiscordWebhook = v
	}
	if v := os.Getenv("SMTP_HOST"); v != "" {
		c.SmtpHost = v
	}
	if v := os.Getenv("SMTP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.SmtpPort = n
		}
	}
	if v := os.Getenv("SMTP_USER"); v != "" {
		c.SmtpUser = v
	}
	if v := os.Getenv("SMTP_PASS"); v != "" {
		c.SmtpPass = v
	}
	if v := os.Getenv("SMTP_FROM"); v != "" {
		c.SmtpFrom = v
	}
	if v := os.Getenv("SMTP_TO"); v != "" {
		c.SmtpTo = v
	}
	if v := os.Getenv("PHONE_WEBHOOK"); v != "" {
		c.PhoneWebhook = v
	}
	if v := os.Getenv("PHONE_TO"); v != "" {
		c.PhoneTo = v
	}
	if v := os.Getenv("COLLECT_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.CollectInterval = d
		}
	}
	if v := os.Getenv("RETENTION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.Retention = d
		}
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		switch v {
		case "debug":
			c.LogLevel = slog.LevelDebug
		case "warn":
			c.LogLevel = slog.LevelWarn
		case "error":
			c.LogLevel = slog.LevelError
		}
	}
	if v := os.Getenv("DEVICE_SWEEP"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.DeviceSweepEnabled = b
		}
	}
}
