package main

import (
	"errors"
	"log"

	"github.com/aymanbagabas/go-nativeclipboard"
)

func main() {
	text, err := nativeclipboard.Text.Read()
	if err != nil {
		// Check for specific error types
		if errors.Is(err, nativeclipboard.ErrUnsupportedPlatform) {
			log.Fatal("This platform does not support clipboard operations")
		} else if errors.Is(err, nativeclipboard.ErrUnavailable) {
			log.Fatal("Clipboard is temporarily unavailable")
		} else if errors.Is(err, nativeclipboard.ErrUnsupported) {
			log.Fatal("The requested format is not supported")
		} else {
			log.Fatalf("Unexpected error: %v", err)
		}
	}

	log.Printf("Clipboard text: %s", string(text))
}
