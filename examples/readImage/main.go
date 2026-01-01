package main

import (
	"log"

	"github.com/aymanbagabas/go-nativeclipboard"
)

func main() {
	image, err := nativeclipboard.Image.Read()
	if err != nil {
		log.Fatalf("Image.Read failed: %v", err)
	}

	log.Printf("Clipboard image: %d bytes", len(image))
}
