// Copyright 2025 Ayman Bagabas
// SPDX-License-Identifier: MIT

//go:build !darwin && !windows && !linux && !freebsd

package nativeclipboard

import (
	"context"
)

func initialize() error {
	return ErrUnsupported
}

func read(t Format) ([]byte, error) {
	return nil, ErrUnsupported
}

func write(t Format, buf []byte) (<-chan struct{}, error) {
	return nil, ErrUnsupported
}

func watch(ctx context.Context, t Format) <-chan []byte {
	ch := make(chan []byte)
	close(ch)
	return ch
}
