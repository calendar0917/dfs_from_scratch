package main

import "testing"

func TestNewVolumeServerForMain_UsesConstructorDefaults(t *testing.T) {
	s := newVolumeServerForMain("vol-test", "")
	if s == nil {
		t.Fatal("newVolumeServerForMain returned nil")
	}
	if s.StorageDir != "./data/vol-test" {
		t.Fatalf("storage dir = %q, want %q", s.StorageDir, "./data/vol-test")
	}
	if s.FileExists("anything") {
		t.Fatal("unexpected file existence check result on fresh server")
	}
	if s.GetFilePath("x") != "data/vol-test/x" {
		t.Fatalf("file path = %q, want %q", s.GetFilePath("x"), "data/vol-test/x")
	}
	if err := s.DeleteFile("missing"); err == nil {
		t.Fatal("DeleteFile on missing file should still return an error")
	}
}

func TestNewVolumeServerForMain_RespectsCustomStorageDir(t *testing.T) {
	s := newVolumeServerForMain("vol-test", "./runtime/custom-vol")
	if s.StorageDir != "./runtime/custom-vol" {
		t.Fatalf("storage dir = %q, want %q", s.StorageDir, "./runtime/custom-vol")
	}
}
