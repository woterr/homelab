// homelab is a lightweight homelab observability daemon.
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"homelab/backend/internal/api"
	"homelab/backend/internal/collect"
	"homelab/backend/internal/config"
	"homelab/backend/internal/devices"
	"homelab/backend/internal/events"
	"homelab/backend/internal/log"
	"homelab/backend/internal/metric"
	"homelab/backend/internal/notify"
	"homelab/backend/internal/rules"
	"homelab/backend/internal/store"
)

func main() {
	cfg := config.New()
	fs := flag.NewFlagSet("homelab", flag.ExitOnError)
	cfg.RegisterFlags(fs)
	_ = fs.Parse(os.Args[1:])
	cfg.Env()

	// Buffered logger keeps the last 1000 lines in memory for the Logs page.
	lg := log.NewBuffered(cfg.LogLevel, os.Stderr, 1000)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		lg.Error("data dir", "error", err)
		os.Exit(1)
	}

	metrics, err := store.OpenMetricStore(ctx, cfg.DBPath(), lg)
	if err != nil {
		lg.Error("metric store", "error", err)
		os.Exit(1)
	}
	defer metrics.Close()

	state, err := store.OpenStateStore(ctx, cfg.DBPath())
	if err != nil {
		lg.Error("state store", "error", err)
		os.Exit(1)
	}
	defer state.Close()

	// In demo mode, seed a realistic event log and a few rules tuned to the
	// simulated waveform so the Events and Rules pages demonstrate live firing.
	if cfg.Demo {
		seedDemo(ctx, state, lg)
		seedDemoLogs(lg)
	}

	bus := events.NewBus()
	tee := &events.Tee{Store: state, Bus: bus}

	// Notification providers. The manager seeds from flags/env and layers
	// persisted settings on top so channels can be configured from the UI.
	nf := notify.NewManager(notify.Config{
		NtfyServer:     cfg.NtfyServer,
		NtfyTopic:      cfg.NtfyTopic,
		DiscordWebhook: cfg.DiscordWebhook,
		SmtpHost:       cfg.SmtpHost,
		SmtpPort:       cfg.SmtpPort,
		SmtpUser:       cfg.SmtpUser,
		SmtpPass:       cfg.SmtpPass,
		SmtpFrom:       cfg.SmtpFrom,
		SmtpTo:         cfg.SmtpTo,
		SmtpInsecure:   cfg.SmtpInsecure,
		PhoneWebhook:   cfg.PhoneWebhook,
		PhoneTo:        cfg.PhoneTo,
	}, state.ListSettings, state.SetSetting)
	if err := nf.Reload(ctx); err != nil {
		lg.Warn("notification config reload failed", "error", err)
	}

	ruleEng := rules.NewEngine(state, tee, nf.Registry(), lg)

	reg := collect.NewRegistry()
	host := collect.Host{Proc: cfg.HostProc, Sys: cfg.HostSys, Docker: cfg.HostDocker, Root: cfg.HostRoot}

	var src api.Sources
	var devMgr *devices.Manager

	if cfg.Demo {
		lg.Info("running in demo mode")
		demo := collect.NewDemoHost()
		reg.Add(demo)
		src = api.Sources{
			System:      demo,
			CPU:         demo,
			Memory:      demo,
			Temperature: demo,
			Network:     demo,
			Disk:        demo,
			Filesystem:  demo,
			Services:    demo,
			Devices:     demo,
			Power:       demo,
		}
	} else {
		// Real collectors; each degrades gracefully when its data source is
		// unavailable (e.g. no Docker socket, no vcgencmd).
		sysCol := collect.NewSystemCollector(host, tee)
		cpuCol := collect.NewCPUCollector(host)
		memCol := collect.NewMemoryCollector(host)
		tempCol := collect.NewTemperatureCollector(host)
		netCol := collect.NewNetworkCollector(host)
		diskCol := collect.NewDiskCollector(host)
		fsCol := collect.NewFilesystemCollector(host)
		svcCol := collect.NewServicesCollector(host, state, tee, lg)
		powerCol := collect.NewPowerCollector(host)

		reg.Add(sysCol)
		reg.Add(cpuCol)
		reg.Add(memCol)
		reg.Add(tempCol)
		reg.Add(netCol)
		reg.Add(diskCol)
		reg.Add(fsCol)
		reg.Add(svcCol)
		reg.Add(powerCol)

		src = api.Sources{
			System:      sysCol,
			CPU:         cpuCol,
			Memory:      memCol,
			Temperature: tempCol,
			Network:     netCol,
			Disk:        diskCol,
			Filesystem:  fsCol,
			Partitions:  fsCol,
			Services:    svcCol,
			Power:       powerCol,
		}

		// Device discovery manager (passive ARP + gentle sweep).
		devMgr = devices.NewManager(state, lg, tee, devices.Options{
			Proc:       cfg.HostProc,
			Sweep:      cfg.DeviceSweepEnabled,
			SweepEvery: cfg.DeviceSweepEvery,
			SweepRetry: cfg.DeviceSweepRetry,
		})
		src.Devices = devMgr
		go devMgr.Run(ctx)
		go func() {
			// Keep the device manager's gateway in sync with the network collector.
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(20 * time.Second):
					_, gw := netCol.Network()
					devMgr.SetGateway(gw)
				}
			}
		}()
	}

	// Scheduler collects on a tick and hands each batch to the hub + rules.
	srv := api.NewServer(cfg, lg, metrics, state, bus, tee, ruleEng, nf, src)
	hub := srv.Hub()

	sched := collect.NewScheduler(reg, cfg.CollectInterval, metrics, tee, lg)
	sched.SetOnTick(func(tctx context.Context, samples []metric.Sample) {
		ruleEng.Evaluate(tctx, samples)
		hub.PushTick(tctx, samples)
	})
	go sched.Run(ctx)

	// Janitor: rollup + retention.
	go janitor(ctx, metrics, cfg, lg)

	// Subscribe the bus to the hub for event push.
	go func() {
		ch, unsub := bus.Subscribe()
		defer unsub()
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-ch:
				hub.PushEvent(ev)
			}
		}
	}()

	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		lg.Info("listening", "addr", cfg.ListenAddr, "demo", cfg.Demo, "data_dir", cfg.DataDir)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			lg.Error("http server", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	lg.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
}

// seedDemo fills the state store with a plausible event log and a small set of
// rules whose thresholds sit inside the demo waveform's range, so they fire
// and recover on their own during a session.
func seedDemo(ctx context.Context, state *store.StateStore, lg *log.Logger) {
	rules := []*store.Rule{
		{ID: "demo.cpu", Name: "CPU sustained high", Metric: "cpu.utilization", Operator: "gt", Threshold: 25, DurationS: 20, Enabled: true, Notify: false, Message: "CPU utilization stayed above 25% for 20s"},
		{ID: "demo.temp", Name: "SoC temperature warm", Metric: "temperature", Operator: "gt", Threshold: 55, DurationS: 20, Enabled: true, Notify: false, Message: "SoC temperature exceeded 55°C for 20s"},
		{ID: "demo.mem", Name: "Memory pressure", Metric: "memory.used_percent", Operator: "gt", Threshold: 62, DurationS: 30, Enabled: true, Notify: false, Message: "Memory usage exceeded 62% for 30s"},
	}
	have := map[string]bool{}
	if existing, err := state.ListRules(ctx); err == nil {
		for _, r := range existing {
			have[r.ID] = true
		}
	}
	seeded := 0
	for _, r := range rules {
		if !have[r.ID] {
			if err := state.UpsertRule(ctx, r); err != nil {
				lg.Warn("seed rule failed", "id", r.ID, "error", err)
				continue
			}
			seeded++
		}
	}
	if seeded > 0 {
		lg.Info("seeded demo rules", "count", seeded)
	}

	existingEvents, err := state.ListEvents(ctx, store.EventFilter{Limit: 1})
	if err != nil || len(existingEvents) > 0 {
		return
	}
	now := time.Now()
	seed := []*events.Event{
		{Type: events.TypeSystemBoot, Severity: events.SeverityInfo, Source: "system", Message: "Node booted; kernel 6.12.75+rpt-rpi-v8 loaded", Time: now.Add(-2 * time.Hour)},
		{Type: events.TypeDeviceJoined, Severity: events.SeverityInfo, Source: "devices", Message: "dev-laptop joined the network (192.168.1.42)", Time: now.Add(-95 * time.Minute)},
		{Type: events.TypeThresholdExceeded, Severity: events.SeverityWarning, Source: "rules", Message: "SoC temperature exceeded 55°C for 20s", Time: now.Add(-52 * time.Minute), Data: map[string]any{"rule": "SoC temperature warm", "metric": "temperature", "value": 58.4, "threshold": 55}},
		{Type: events.TypeNotificationSent, Severity: events.SeverityInfo, Source: "rules", Message: "Notification sent via ntfy for SoC temperature warm", Time: now.Add(-52 * time.Minute), Data: map[string]any{"provider": "ntfy", "rule": "SoC temperature warm"}},
		{Type: events.TypeThresholdRecovered, Severity: events.SeverityInfo, Source: "rules", Message: "SoC temperature warm recovered (temperature = 51.2)", Time: now.Add(-48 * time.Minute), Data: map[string]any{"rule": "SoC temperature warm", "metric": "temperature", "value": 51.2}},
		{Type: events.TypeDeviceLeft, Severity: events.SeverityInfo, Source: "devices", Message: "tv left the network (192.168.1.200)", Time: now.Add(-70 * time.Minute)},
		{Type: events.TypeServiceStopped, Severity: events.SeverityWarning, Source: "services", Message: "smartd service went inactive", Time: now.Add(-20 * time.Minute)},
		{Type: events.TypeThresholdExceeded, Severity: events.SeverityWarning, Source: "rules", Message: "CPU utilization stayed above 25% for 20s", Time: now.Add(-6 * time.Minute), Data: map[string]any{"rule": "CPU sustained high", "metric": "cpu.utilization", "value": 27.9, "threshold": 25}},
	}
	for _, ev := range seed {
		if err := state.InsertEvent(ctx, ev); err != nil {
			lg.Warn("seed event failed", "error", err)
		}
	}
	lg.Info("seeded demo events", "count", len(seed))
}

// seedDemoLogs writes a plausible boot/interaction trail through the real
// logger so the Logs page has context to show in demo mode.
func seedDemoLogs(lg *log.Logger) {
	now := time.Now()
	lg.Info("node booted", "kernel", "6.12.75+rpt-rpi-v8", "arch", "aarch64", "uptime", "3d")
	lg.Info("starting collectors", "interval", "5s", "sources", "system,cpu,memory,temp,net,disk,fs,services")
	lg.Info("websocket hub ready", "clients", 0)
	lg.Info("notification registry initialized", "providers", "ntfy,discord,email,phone,telegram")
	lg.Warn("temperature above nominal", "zone", "cpu-thermal", "temp_c", 58.4, "threshold", 55)
	lg.Info("threshold recovered", "rule", "SoC temperature warm", "temp_c", 51.2)
	lg.Warn("device disappeared", "ip", "192.168.1.200", "mac", "aa:bb:cc:dd:ee:02")
	lg.Info("service changed state", "service", "smartd", "state", "inactive")
	lg.Error("metric write failed", "series", "disk.io_write", "error", "storage full", "retry_in", "30s")
	lg.Info("metric write recovered", "series", "disk.io_write")
	lg.Info("http server listening", "addr", ":8080", "demo", true, "since", now.Format(time.RFC3339))
}

// janitor periodically folds old raw samples into rollups and trims data past
// the retention window.
func janitor(ctx context.Context, ms store.MetricStore, cfg *config.Config, lg *log.Logger) {
	t := time.NewTicker(cfg.RollupInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cutoff := time.Now().Add(-cfg.RollupWindow)
			if err := ms.Rollup(ctx, cutoff); err != nil {
				lg.Warn("rollup failed", "error", err)
			}
			if err := ms.Trim(ctx, cfg.Retention); err != nil {
				lg.Warn("trim failed", "error", err)
			}
		}
	}
}
