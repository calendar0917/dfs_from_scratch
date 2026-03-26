package service

import (
	"testing"
	"time"
)

func TestNewForwardPoolConfig_UsesSecondTimeout(t *testing.T) {
	cfg := newForwardPoolConfig()
	if cfg.ConnTimeout != 5*time.Second {
		t.Fatalf("ConnTimeout = %v, want %v", cfg.ConnTimeout, 5*time.Second)
	}
}

func TestForwardStreamTimeout_UsesSeconds(t *testing.T) {
	if defaultForwardStreamTimeout != 5*time.Second {
		t.Fatalf("forward stream timeout = %v, want %v", defaultForwardStreamTimeout, 5*time.Second)
	}
}
