# go-nativeclipboard

<p>

[![Go Reference](https://pkg.go.dev/badge/github.com/aymanbagabas/go-nativeclipboard.svg)](https://pkg.go.dev/github.com/aymanbagabas/go-nativeclipboard)
[![Go Report Card](https://goreportcard.com/badge/github.com/aymanbagabas/go-nativeclipboard)](https://goreportcard.com/report/github.com/aymanbagabas/go-nativeclipboard)
[![build](https://github.com/aymanbagabas/go-nativeclipboard/actions/workflows/build.yml/badge.svg?branch=main)](https://github.com/aymanbagabas/go-nativeclipboard/actions/workflows/build.yml)
[![Latest Release](https://img.shields.io/github/v/release/aymanbagabas/go-nativeclipboard)](https://github.com/aymanbagabas/go-nativeclipboard/releases/latest)

</p>

A cross-platform clipboard library for Go that works without cgo. This is a port of [golang.design/x/clipboard](https://github.com/golang-design/clipboard) that uses [purego](https://github.com/ebitengine/purego) instead of cgo, enabling clipboard access without a C compiler.

## Why?

The original [golang.design/x/clipboard](https://github.com/golang-design/clipboard) is excellent but requires cgo, which complicates cross-compilation and slows down builds. This library provides the same functionality using pure Go with purego for native OS API calls.

**Benefits:**

- No C compiler needed
- Faster builds
- Easy cross-compilation
- Smaller binaries
- Same familiar API

## Installation

```bash
go get github.com/aymanbagabas/go-nativeclipboard
```

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    "github.com/aymanbagabas/go-nativeclipboard"
)

func main() {
    // Write text to clipboard
    _, err := nativeclipboard.Text.Write([]byte("Hello, clipboard!"))
    if err != nil {
        log.Fatal(err)
    }

    // Read text from clipboard
    data, err := nativeclipboard.Text.Read()
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(string(data))
}
```

## Usage

### Reading and Writing

```go
// Text clipboard
data, err := nativeclipboard.Text.Read()
changed, err := nativeclipboard.Text.Write([]byte("hello"))

// Image clipboard (PNG format)
imageData, _ := os.ReadFile("image.png")
changed, err := nativeclipboard.Image.Write(imageData)
data, err := nativeclipboard.Image.Read()
```

### Watching for Changes

Monitor clipboard changes in real-time:

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

ch, err := nativeclipboard.Text.Watch(ctx)
if err != nil {
    log.Fatal(err)
}

for data := range ch {
    fmt.Println("Clipboard changed:", string(data))
}
```

### Change Notifications

Get notified when your clipboard data is overwritten:

```go
changed, err := nativeclipboard.Text.Write([]byte("Hello"))
if err != nil {
    log.Fatal(err)
}

select {
case <-changed:
    fmt.Println("Clipboard was overwritten by another application")
case <-time.After(5 * time.Second):
    fmt.Println("Clipboard content still intact")
}
```

## API Reference

The library provides a simple `Format` type with methods:

```go
type Format int

const (
    Text  Format = iota  // UTF-8 text
    Image                // PNG image
)

// Methods
func (f Format) Read() ([]byte, error)
func (f Format) Write(buf []byte) (<-chan struct{}, error)
func (f Format) Watch(ctx context.Context) (<-chan []byte, error)
```

Use the pre-defined constants:

- `nativeclipboard.Text` for text operations
- `nativeclipboard.Image` for image operations

## Platform Support

| Platform        | Status      | Implementation                   |
| --------------- | ----------- | -------------------------------- |
| macOS           | ✅ Complete | NSPasteboard via purego/objc     |
| Linux (X11)     | ✅ Complete | X11 via purego (requires libX11) |
| Linux (Wayland) | 🚧 Planned  | Not yet implemented              |
| Windows         | ✅ Complete | Win32 API via syscall            |
| FreeBSD         | ✅ Complete | X11 via purego (requires libX11) |
| Other platforms | ❌ Unsupported | Returns `ErrUnavailable` error  |

**Note:** On unsupported platforms (OpenBSD, NetBSD, Solaris, iOS, Android, etc.), the package compiles successfully but all clipboard operations return `ErrUnavailable`.

### Linux Requirements

**X11 (supported):** You need libX11 installed:

```bash
# Debian/Ubuntu
sudo apt install libx11-dev

# Fedora/RHEL
sudo dnf install libX11-devel

# For headless environments
sudo apt install xvfb
Xvfb :99 -screen 0 1024x768x24 > /dev/null 2>&1 &
export DISPLAY=:99.0
```

**Wayland (not yet supported):** Wayland clipboard support is planned but not yet implemented. If you're using Wayland, you'll need XWayland compatibility layer with X11 libraries installed for the clipboard to work.

### FreeBSD Requirements

**X11 (supported):** You need libX11 installed:

```bash
# FreeBSD
sudo pkg install xorg-libraries

# For headless environments
sudo pkg install xorg-vfbserver
Xvfb :99 -screen 0 1024x768x24 > /dev/null 2>&1 &
export DISPLAY=:99.0
```

**Building on FreeBSD:** When building with `CGO_ENABLED=0` (the default for this library), you need to add special gcflags:

```bash
go build -gcflags="github.com/ebitengine/purego/internal/fakecgo=-std" ./...
go test -gcflags="github.com/ebitengine/purego/internal/fakecgo=-std" ./...
```

**Note:** FreeBSD typically installs X11 libraries to `/usr/local/lib` or `/usr/X11R6/lib`. The library will automatically search these common FreeBSD locations.

## How It Works

This library uses different approaches per platform:

- **macOS**: Calls Objective-C runtime and AppKit (NSPasteboard) using purego/objc
- **Linux (X11)**: Dynamically loads libX11.so and calls X11 clipboard functions via purego
- **FreeBSD**: Same X11 implementation as Linux, with automatic detection of FreeBSD-specific library paths
- **Linux (Wayland)**: Not yet implemented - planned for future release
- **Windows**: Uses Win32 clipboard API (user32.dll, kernel32.dll) via Go's syscall package
- **Other platforms**: Stub implementation that returns `ErrUnavailable` for all operations

All approaches avoid cgo entirely, resulting in faster builds and easier cross-compilation.

## Testing

```bash
# Run tests
go test -v

# Cross-compile (no C compiler needed!)
CGO_ENABLED=0 GOOS=linux go build
CGO_ENABLED=0 GOOS=windows go build
```

## Contributing

Contributions welcome! Some ideas:

- Wayland clipboard support for Linux
- Additional clipboard formats (HTML, RTF, files)
- Performance optimizations
- Better error messages

## License

MIT License - see [LICENSE](./LICENSE) file for details.

## Credits

- This is a port of [golang.design/x/clipboard](https://github.com/golang-design/clipboard) by Changkun Ou
- Built with [purego](https://github.com/ebitengine/purego) by Ebitengine
- Thanks to the Go community for making cgo-free native calls possible

## See Also

- [golang.design/x/clipboard](https://github.com/golang-design/clipboard) - Original cgo-based implementation
- [purego](https://github.com/ebitengine/purego) - Pure Go library for calling native APIs
