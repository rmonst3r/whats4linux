package socket

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func useTestSocketPath(t *testing.T) {
	t.Helper()
	original := UDSPath
	UDSPath = filepath.Join(t.TempDir(), "whats4linux.sock")
	t.Cleanup(func() { UDSPath = original })
}

func TestCloseStopsListenerAndRemovesSocket(t *testing.T) {
	useTestSocketPath(t)
	s, err := NewUnixSocket(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- s.ListenAndServe() }()

	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(UDSPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("socket listener did not start")
		}
		time.Sleep(time.Millisecond)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("socket listener did not stop")
	}
	if _, err := os.Stat(UDSPath); !os.IsNotExist(err) {
		t.Fatalf("socket path remains after close: %v", err)
	}
}

func TestCloseBeforeListenDoesNotLeaveSocket(t *testing.T) {
	useTestSocketPath(t)
	s, err := NewUnixSocket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.ListenAndServe(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(UDSPath); !os.IsNotExist(err) {
		t.Fatalf("socket path remains after closed server attempted to listen: %v", err)
	}
}
