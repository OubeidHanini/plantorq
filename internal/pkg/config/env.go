// Package config reads service settings from environment variables, following
// twelve-factor conventions so the same image runs in Compose and Kubernetes.
package config

import (
	"os"
	"strconv"
	"time"
)

// String returns the value of key, or def when it is unset or empty.
func String(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Int returns key parsed as an int, or def when unset or invalid.
func Int(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

// Duration returns key parsed with time.ParseDuration, or def.
func Duration(key string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(key)); err == nil {
		return v
	}
	return def
}
