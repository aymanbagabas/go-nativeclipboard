// Copyright 2025 Ayman Bagabas
// SPDX-License-Identifier: MIT

//go:build !(darwin || linux || freebsd || windows) || ios || android

package nativeclipboard

import (
	"context"
)

func initialize() error {
	return ErrUnsupportedPlatform
}

func read(t Format) ([]byte, error) {
	return nil, ErrUnsupportedPlatform
}

func write(t Format, buf []byte) (<-chan struct{}, error) {
	return nil, ErrUnsupportedPlatform
}

func watch(ctx context.Context, t Format) <-chan []byte {
	ch := make(chan []byte)
	close(ch)
	return ch
}
