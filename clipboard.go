// Package nativeclipboard provides cross-platform clipboard access using purego
// instead of cgo. It supports text and image data on macOS, Linux, and Windows.
//
// The package initializes automatically and returns errors from individual operations.
//
// Read and write clipboard data:
//
//	// Write text to clipboard
//	changed, err := nativeclipboard.Text.Write([]byte("hello world"))
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Read text from clipboard
//	data, err := nativeclipboard.Text.Read()
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println(string(data))
//
// The package supports text (UTF-8) and images (PNG).
package nativeclipboard

import (
	"context"
	"errors"
	"sync"
)

var (
	// ErrUnavailable indicates the clipboard is not available
	ErrUnavailable = errors.New("clipboard unavailable")
	// ErrUnsupported indicates the requested format is not supported
	ErrUnsupported = errors.New("unsupported format")
	// ErrUnsupportedPlatform indicates the platform does not support clipboard operations
	ErrUnsupportedPlatform = errors.New("unsupported platform")
)

// Format represents a clipboard data format.
type Format int

// Supported clipboard formats
const (
	// Text provides access to text clipboard operations.
	Text Format = iota
	// Image provides access to image (PNG) clipboard operations.
	Image
)

var (
	// Due to platform limitations, concurrent reads can cause issues.
	// Use a global lock to guarantee one operation at a time.
	lock      = sync.Mutex{}
	initError error
)

func init() {
	initError = initialize()
}

// Read reads clipboard data in this format.
// Returns an error if the clipboard is unavailable or initialization failed.
func (f Format) Read() ([]byte, error) {
	if initError != nil {
		return nil, initError
	}

	lock.Lock()
	defer lock.Unlock()

	buf, err := read(f)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// Write writes data to the clipboard in this format.
// Returns a channel that receives a signal when the clipboard content
// has been overwritten by another application, and an error if the operation fails.
func (f Format) Write(buf []byte) (<-chan struct{}, error) {
	if initError != nil {
		return nil, initError
	}

	lock.Lock()
	defer lock.Unlock()

	changed, err := write(f, buf)
	if err != nil {
		return nil, err
	}
	return changed, nil
}

// Watch returns a channel that receives clipboard data whenever it changes.
// The channel will be closed when the provided context is canceled.
// Returns an error if the clipboard is unavailable or initialization failed.
func (f Format) Watch(ctx context.Context) (<-chan []byte, error) {
	if initError != nil {
		return nil, initError
	}
	return watch(ctx, f), nil
}
