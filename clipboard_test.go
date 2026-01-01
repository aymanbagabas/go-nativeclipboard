package nativeclipboard

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"
)

// createTestPNG creates a simple test PNG image and returns it as PNG-encoded bytes
func createTestPNG(width, height int, c color.Color) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func TestInit(t *testing.T) {
	// Init happens automatically, just check for errors
	if initError != nil {
		t.Fatalf("Init failed: %v", initError)
	}
}

func TestWriteReadText(t *testing.T) {
	testData := []byte("Hello, clipboard!")
	
	// Write text to clipboard
	ch, err := Text.Write(testData)
	if err != nil {
		t.Fatalf("Text.Write failed: %v", err)
	}
	if ch == nil {
		t.Fatal("Text.Write returned nil channel")
	}

	// Give the system a moment to process
	time.Sleep(100 * time.Millisecond)

	// Read text from clipboard
	data, err := Text.Read()
	if err != nil {
		t.Fatalf("Text.Read failed: %v", err)
	}
	if data == nil {
		t.Fatal("Text.Read returned nil")
	}

	if string(data) != string(testData) {
		t.Fatalf("Expected %q, got %q", testData, data)
	}
}

func TestWriteReadImage(t *testing.T) {
	// Create a small red test image
	testImage, err := createTestPNG(10, 10, color.RGBA{255, 0, 0, 255})
	if err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}

	// Write image to clipboard
	ch, err := Image.Write(testImage)
	if err != nil {
		t.Fatalf("Image.Write failed: %v", err)
	}
	if ch == nil {
		t.Fatal("Image.Write returned nil channel")
	}

	// Give the system a moment to process
	time.Sleep(100 * time.Millisecond)

	// Read image from clipboard
	data, err := Image.Read()
	if err != nil {
		t.Fatalf("Image.Read failed: %v", err)
	}
	if data == nil {
		t.Fatal("Image.Read returned nil")
	}

	// Verify it's valid PNG data by decoding
	_, err = png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Image.Read returned invalid PNG data: %v", err)
	}

	// Note: We don't compare exact bytes because different platforms
	// may encode the same image slightly differently
	if len(data) == 0 {
		t.Fatal("Image.Read returned empty data")
	}
}

func TestWriteEmptyText(t *testing.T) {
	// Write empty text
	_, err := Text.Write([]byte{})
	if err != nil {
		t.Fatalf("Text.Write failed: %v", err)
	}
	
	time.Sleep(100 * time.Millisecond)

	// Read may return empty, nil, or error (platform dependent)
	data, _ := Text.Read()
	if data != nil && len(data) > 0 {
		t.Fatalf("Expected empty clipboard, got %q", data)
	}
}

func TestWatchText(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start watching
	ch, err := Text.Watch(ctx)
	if err != nil {
		t.Fatalf("Text.Watch failed: %v", err)
	}
	
	// Give watch goroutine time to start
	time.Sleep(200 * time.Millisecond)

	// Write something to trigger watch
	testData := []byte("Watch test")
	_, err = Text.Write(testData)
	if err != nil {
		t.Fatalf("Text.Write failed: %v", err)
	}

	select {
	case data := <-ch:
		if string(data) != string(testData) {
			t.Fatalf("Expected %q, got %q", testData, data)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for clipboard change")
	}
}

func TestWatchImage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start watching
	ch, err := Image.Watch(ctx)
	if err != nil {
		t.Fatalf("Image.Watch failed: %v", err)
	}
	
	// Give watch goroutine time to start
	time.Sleep(200 * time.Millisecond)

	// Write an image to trigger watch
	testImage, err := createTestPNG(5, 5, color.RGBA{0, 255, 0, 255})
	if err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}

	_, err = Image.Write(testImage)
	if err != nil {
		t.Fatalf("Image.Write failed: %v", err)
	}

	select {
	case data := <-ch:
		if len(data) == 0 {
			t.Fatal("Watch returned empty image data")
		}
		// Verify it's valid PNG
		_, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("Watch returned invalid PNG data: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for clipboard change")
	}
}

func TestMultipleReads(t *testing.T) {
	testData := []byte("Multiple reads test")
	_, err := Text.Write(testData)
	if err != nil {
		t.Fatalf("Text.Write failed: %v", err)
	}
	
	time.Sleep(100 * time.Millisecond)

	// Read multiple times
	for i := 0; i < 5; i++ {
		data, err := Text.Read()
		if err != nil {
			t.Fatalf("Text.Read %d failed: %v", i, err)
		}
		if data == nil {
			t.Fatalf("Text.Read %d returned nil", i)
		}
		if string(data) != string(testData) {
			t.Fatalf("Text.Read %d: expected %q, got %q", i, testData, data)
		}
	}
}

func TestMultipleImageReads(t *testing.T) {
	// Create test image
	testImage, err := createTestPNG(8, 8, color.RGBA{0, 0, 255, 255})
	if err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}

	_, err = Image.Write(testImage)
	if err != nil {
		t.Fatalf("Image.Write failed: %v", err)
	}
	
	time.Sleep(100 * time.Millisecond)

	// Read multiple times
	for i := 0; i < 3; i++ {
		data, err := Image.Read()
		if err != nil {
			t.Fatalf("Image.Read %d failed: %v", i, err)
		}
		if data == nil {
			t.Fatalf("Image.Read %d returned nil", i)
		}
		// Verify valid PNG
		_, err = png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("Image.Read %d returned invalid PNG: %v", i, err)
		}
	}
}

