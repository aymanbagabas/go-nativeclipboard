package main

import (
	"log"

	"github.com/aymanbagabas/go-nativeclipboard"
)

func main() {
	text, err := nativeclipboard.Text.Read()
	if err != nil {
		log.Fatalf("Text.Read failed: %v", err)
	}

	log.Printf("Clipboard text: %s", string(text))
}
