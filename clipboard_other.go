// Copyright 2025 Ayman Bagabas
// SPDX-License-Identifier: MIT

//go:build !(darwin && !ios) && !windows && !(linux && !android) && !(freebsd && !android)

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
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch
}
