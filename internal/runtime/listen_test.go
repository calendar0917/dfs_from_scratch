package runtime

import (
	"errors"
	"net"
	"testing"
)

type listenFunc func(network, address string) (net.Listener, error)

func TestMustListen_ReturnsListener(t *testing.T) {
	stub := func(network, address string) (net.Listener, error) {
		return &net.TCPListener{}, nil
	}
	got, err := MustListen(stub, "tcp", ":50052")
	if err != nil {
		t.Fatalf("MustListen returned error: %v", err)
	}
	if got == nil {
		t.Fatal("MustListen returned nil listener")
	}
}

func TestMustListen_ReturnsWrappedError(t *testing.T) {
	wantErr := errors.New("boom")
	stub := func(network, address string) (net.Listener, error) {
		return nil, wantErr
	}
	got, err := MustListen(stub, "tcp", ":50052")
	if err == nil {
		t.Fatal("MustListen error = nil, want non-nil")
	}
	if got != nil {
		t.Fatal("MustListen listener should be nil on error")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("MustListen error = %v, want wrapped %v", err, wantErr)
	}
}
