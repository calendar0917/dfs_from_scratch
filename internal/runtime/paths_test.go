package runtime

import "testing"

func TestResolveMasterPersistPath_Default(t *testing.T) {
	got := ResolveMasterPersistPath("")
	want := "persist.log"
	if got != want {
		t.Fatalf("default persist path = %q, want %q", got, want)
	}
}

func TestResolveMasterPersistPath_Custom(t *testing.T) {
	got := ResolveMasterPersistPath("./runtime/master/meta.json")
	want := "./runtime/master/meta.json"
	if got != want {
		t.Fatalf("custom persist path = %q, want %q", got, want)
	}
}

func TestResolveVolumeStorageDir_Default(t *testing.T) {
	got := ResolveVolumeStorageDir("vol-9", "")
	want := "./data/vol-9"
	if got != want {
		t.Fatalf("default storage dir = %q, want %q", got, want)
	}
}

func TestResolveVolumeStorageDir_Custom(t *testing.T) {
	got := ResolveVolumeStorageDir("vol-9", "./runtime/vol-9")
	want := "./runtime/vol-9"
	if got != want {
		t.Fatalf("custom storage dir = %q, want %q", got, want)
	}
}

func TestResolveVolumeStorageDir_UsesNodeIDWhenMissing(t *testing.T) {
	got := ResolveVolumeStorageDir("", "")
	want := "./data/volume"
	if got != want {
		t.Fatalf("empty node id storage dir = %q, want %q", got, want)
	}
}
