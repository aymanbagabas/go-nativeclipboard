// Copyright 2025 Ayman Bagabas
// SPDX-License-Identifier: MIT

//go:build linux && !android

package nativeclipboard

import (
	"context"
)

// read is the platform entry point for reading clipboard
func read(t Format) ([]byte, error) {
	return readLinux(t)
}

// write is the platform entry point for writing clipboard
func write(t Format, buf []byte) (<-chan struct{}, error) {
	return writeLinux(t, buf)
}

// watch is the platform entry point for watching clipboard
func watch(ctx context.Context, t Format) <-chan []byte {
	return watchLinux(ctx, t)
}

// initialize is the platform entry point for initialization
func initialize() error {
	return initializeLinux()
}

// initializeLinux tries to initialize the clipboard system
// It will be implemented by either X11 or Wayland depending on session
func initializeLinux() error {
	// Try Wayland first if in Wayland session
	if isWaylandSession() {
		if err := initializeWayland(); err == nil {
			return nil
		}
	}
	// Fall back to X11
	return initializeX11()
}

// readLinux routes to the appropriate implementation
func readLinux(t Format) ([]byte, error) {
	if isWaylandSession() {
		data, err := readWayland(t)
		if err == nil {
			return data, nil
		}
		// Fallback to X11 if Wayland fails (XWayland)
	}
	return readX11(mimeTypeForFormat(t))
}

// writeLinux routes to the appropriate implementation
func writeLinux(t Format, buf []byte) (<-chan struct{}, error) {
	if isWaylandSession() {
		ch, err := writeWayland(t, buf)
		if err == nil {
			return ch, nil
		}
		// Fallback to X11 if Wayland fails (XWayland)
	}
	return writeX11(mimeTypeForFormat(t), buf)
}

// watchLinux routes to the appropriate implementation
func watchLinux(ctx context.Context, t Format) <-chan []byte {
	if isWaylandSession() {
		ch := watchWayland(ctx, t)
		// Try to check if channel is immediately closed
		select {
		case _, ok := <-ch:
			if !ok {
				// Fallback to X11
				return watchX11(ctx, t)
			}
			// Can't put value back, so create new channel and forward
			newCh := make(chan []byte, 1)
			go func() {
				defer close(newCh)
				// Forward all values from ch
				for data := range ch {
					newCh <- data
				}
			}()
			return newCh
		default:
			// Channel is still open, use it
			return ch
		}
	}
	return watchX11(ctx, t)
}

func mimeTypeForFormat(t Format) string {
	switch t {
	case Text:
		return "UTF8_STRING"
	case Image:
		return "image/png"
	default:
		return ""
	}
}
