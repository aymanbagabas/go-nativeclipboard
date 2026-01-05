//go:build !(darwin && !ios) && !windows && !(linux && !android) && !(freebsd && !android)

package nativeclipboard

import (
	"context"
	"testing"
	"time"
)

// TestStubInitialize verifies that initialize returns ErrUnsupported on unsupported platforms
func TestStubInitialize(t *testing.T) {
	err := initialize()
	if err != ErrUnsupported {
		t.Errorf("Expected ErrUnsupported, got %v", err)
	}
}

// TestStubRead verifies that read returns ErrUnsupported on unsupported platforms
func TestStubRead(t *testing.T) {
	data, err := read(Text)
	if err != ErrUnsupported {
		t.Errorf("Expected ErrUnsupported, got %v", err)
	}
	if data != nil {
		t.Errorf("Expected nil data, got %v", data)
	}
}

// TestStubWrite verifies that write returns ErrUnsupported on unsupported platforms
func TestStubWrite(t *testing.T) {
	ch, err := write(Text, []byte("test"))
	if err != ErrUnsupported {
		t.Errorf("Expected ErrUnsupported, got %v", err)
	}
	if ch != nil {
		t.Errorf("Expected nil channel, got %v", ch)
	}
}

// TestStubWatch verifies that watch returns a channel that closes when context is canceled on unsupported platforms
func TestStubWatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ch := watch(ctx, Text)
	if ch == nil {
		t.Fatal("Expected non-nil channel")
	}

	// Channel should close when context is done
	select {
	case data, ok := <-ch:
		if ok {
			t.Errorf("Expected channel to be closed, but received data: %v", data)
		}
		// Channel closed as expected
	case <-time.After(200 * time.Millisecond):
		t.Error("Expected channel to be closed after context timeout")
	}
}
