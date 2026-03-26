package service

import "testing"

func TestNewVolumeServer_InitializesConnectionPool(t *testing.T) {
	s := NewVolumeServer("./data/test")
	if s == nil {
		t.Fatal("NewVolumeServer returned nil")
	}
	if s.connPool == nil {
		t.Fatal("connPool should be initialized")
	}
}

func TestZeroValueVolumeServer_HasNilConnectionPool(t *testing.T) {
	var s VolumeServer
	if s.connPool != nil {
		t.Fatal("zero-value connPool should be nil")
	}
}
