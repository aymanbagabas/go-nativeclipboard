// Copyright 2025 Ayman Bagabas
// SPDX-License-Identifier: MIT

//go:build !(darwin || linux || freebsd || windows) || ios || android

package nativeclipboard

import (
	"errors"
	"testing"
)

func TestUnsupportedPlatformError(t *testing.T) {
	// Test that initialize returns ErrUnsupportedPlatform
	if !errors.Is(initError, ErrUnsupportedPlatform) {
		t.Errorf("Expected ErrUnsupportedPlatform, got %v", initError)
	}

	// Test that read returns ErrUnsupportedPlatform
	_, err := Text.Read()
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("Expected ErrUnsupportedPlatform from Read, got %v", err)
	}

	// Test that write returns ErrUnsupportedPlatform
	_, err = Text.Write([]byte("test"))
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("Expected ErrUnsupportedPlatform from Write, got %v", err)
	}
}

func TestErrorDistinction(t *testing.T) {
	// Verify that ErrUnsupportedPlatform is distinct from ErrUnavailable
	if errors.Is(ErrUnsupportedPlatform, ErrUnavailable) {
		t.Error("ErrUnsupportedPlatform should not be equal to ErrUnavailable")
	}

	// Verify that ErrUnsupportedPlatform is distinct from ErrUnsupported
	if errors.Is(ErrUnsupportedPlatform, ErrUnsupported) {
		t.Error("ErrUnsupportedPlatform should not be equal to ErrUnsupported")
	}
}
