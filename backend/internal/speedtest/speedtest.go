// Package speedtest measures real WAN throughput against Cloudflare's public
// speed-test endpoints, replacing simulated gauges with genuine measurements.
package speedtest

import (
	"bytes"
	"context"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Result is the outcome of one speed test run.
type Result struct {
	DownloadMbps float64   `json:"download_mbps"`
	UploadMbps   float64   `json:"upload_mbps"`
	LatencyMs    float64   `json:"latency_ms"`
	Error        string    `json:"error,omitempty"`
	TestedAt     time.Time `json:"tested_at"`
}

const (
	endpoint    = "https://speed.cloudflare.com"
	minElapsed  = 100 * time.Millisecond
	concurrency = 6
)

var sizes = []int64{100 * 1024, 1024 * 1024, 10 * 1024 * 1024, 25 * 1024 * 1024}

// Run executes a full test. Each phase is bounded by ctx and the per-request
// client timeout; failures degrade gracefully into an Error result instead of
// hanging the dashboard.
func Run(ctx context.Context, client *http.Client) Result {
	res := Result{TestedAt: time.Now()}
	lat, err := probeLatency(ctx, client)
	if err != nil {
		res.Error = "latency probe failed: " + err.Error()
		return res
	}
	res.LatencyMs = lat
	if err := ctx.Err(); err != nil {
		res.Error = err.Error()
		return res
	}
	down, err := measure(ctx, client, false)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.DownloadMbps = down
	up, err := measure(ctx, client, true)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.UploadMbps = up
	return res
}

// probeLatency returns the best round-trip of three small requests.
func probeLatency(ctx context.Context, client *http.Client) (float64, error) {
	best := math.Inf(1)
	found := false
	for i := 0; i < 3; i++ {
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint+"/__down?bytes=0", nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			continue
		}
		ms := float64(time.Since(start).Microseconds()) / 1000
		if ms < best {
			best = ms
		}
		found = true
	}
	if !found {
		return 0, io.ErrUnexpectedEOF
	}
	return best, nil
}

// measure runs the download (GET /__down?bytes=N) or upload (POST /__up)
// phase across increasing sizes and parallel streams, keeping the best rate.
func measure(ctx context.Context, client *http.Client, upload bool) (float64, error) {
	best := 0.0
	gotAny := false
	for _, size := range sizes {
		var payload []byte
		if upload {
			payload = randomPayload(size)
		}
		rates := runPhase(ctx, client, size, payload)
		for _, r := range rates {
			gotAny = true
			if r > best {
				best = r
			}
		}
		if ctx.Err() != nil {
			break
		}
	}
	if !gotAny {
		return 0, io.ErrUnexpectedEOF
	}
	return best, nil
}

// runPhase issues `concurrency` parallel transfers of the given size and
// returns per-stream throughput in Mbps for the ones that completed.
func runPhase(ctx context.Context, client *http.Client, size int64, payload []byte) []float64 {
	url := endpoint + "/__down?bytes=" + strconv.FormatInt(size, 10)
	rates := make([]float64, 0, concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			start := time.Now()
			var err error
			var n int64
			if payload != nil {
				req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/__up", bytes.NewReader(payload))
				if rerr != nil {
					return
				}
				req.Header.Set("Content-Type", "application/octet-stream")
				var resp *http.Response
				resp, err = client.Do(req)
				if err == nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					n = size
				}
			} else {
				var req *http.Request
				req, err = http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
				if err != nil {
					return
				}
				var resp *http.Response
				resp, err = client.Do(req)
				if err == nil {
					n, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			}
			if err != nil {
				return
			}
			el := time.Since(start)
			if el < minElapsed {
				return
			}
			mbps := float64(n) * 8 / el.Seconds() / 1e6
			mu.Lock()
			rates = append(rates, mbps)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return rates
}

func strconvFormatInt(n int64) string {
	return strconv.FormatInt(n, 10)
}

var rndPool = sync.Pool{New: func() any { return rand.New(rand.NewSource(time.Now().UnixNano())) }}

// randomPayload returns a deterministic-looking buffer of the given size using
// a per-call RNG so payloads differ between requests but stay cheap to build.
func randomPayload(size int64) []byte {
	buf := make([]byte, size)
	r := rndPool.Get().(*rand.Rand)
	defer rndPool.Put(r)
	// Fill in 64KB chunks so large payloads don't stall the main loop.
	chunk := make([]byte, 64*1024)
	for off := 0; off < len(buf); {
		r.Read(chunk)
		n := copy(buf[off:], chunk)
		off += n
	}
	return buf
}
