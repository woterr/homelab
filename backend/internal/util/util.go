// Package util contains small shared helpers.
package util

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

// ReadFirstLine returns the first line of a file, trimmed, or "" on error.
func ReadFirstLine(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := string(b)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// ParseFloatOr parses s as float64, returning def on error.
func ParseFloatOr(s string, def float64) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return def
	}
	return f
}

// ParseIntOr parses s as int64, returning def on error.
func ParseIntOr(s string, def int64) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return def
	}
	return v
}

// Arch returns a normalized architecture name.
func Arch() string {
	switch runtime.GOARCH {
	case "arm64", "aarch64":
		return "aarch64"
	case "amd64":
		return "x86_64"
	default:
		return runtime.GOARCH
	}
}
