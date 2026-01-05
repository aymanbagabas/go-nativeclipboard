# AGENTS.md

This file documents everything an AI agent needs to know to work effectively in this codebase.

## Project Overview

**go-nativeclipboard** is a Go library that provides native clipboard functionality using [purego](https://github.com/ebitengine/purego) instead of cgo. This enables clipboard access without requiring a C compiler.

- **Language**: Go 1.25.4
- **Module**: `github.com/aymanbagabas/go-nativeclipboard`
- **Key Dependencies**: 
  - `github.com/ebitengine/purego` - For calling native APIs without cgo
- **Status**: All platform implementations complete (macOS, Windows, FreeBSD, Linux X11, and Linux Wayland)

## Project Structure

```
.
├── clipboard.go          # Main API and public interfaces
├── clipboard_darwin.go   # macOS implementation (NSPasteboard via purego)
├── clipboard_linux.go    # Linux routing layer (Wayland/X11 detection and fallback)
├── clipboard_wayland.go  # Linux Wayland implementation (zwlr_data_control via purego)
├── clipboard_x11.go      # Linux X11 implementation (X11 via purego)
├── clipboard_freebsd.go  # FreeBSD implementation (X11 via purego)
├── clipboard_windows.go  # Windows implementation (Win32 API via syscall)
├── clipboard_test.go     # Platform-agnostic tests
├── doc.go               # Package documentation for pkg.go.dev
├── go.mod               # Go module definition
├── go.sum               # Dependency checksums
├── README.md            # User-facing documentation
├── AGENTS.md            # This file
└── examples/            # Example programs
```

## Dependencies

The project uses purego to call native operating system APIs without cgo:

1. **github.com/ebitengine/purego** - Pure Go library for calling C functions and native APIs
   - Used to call Objective-C runtime on macOS
   - Will be used for X11/Wayland on Linux and Win32 on Windows

Previous dependencies (from golang.design/x/clipboard) have been removed as they used cgo.

## Essential Commands

### Building
```bash
go build ./...
```

### Testing
```bash
# Run all tests on Linux/BSD (requires X11 and Xvfb for headless)
go test ./...

# Run specific test
go test -v -run TestWriteReadText

# Run with coverage
go test -cover ./...

# Run with race detector
go test -race ./...

# For headless Linux/BSD testing
Xvfb :99 -screen 0 1024x768x24 > /dev/null 2>&1 &
export DISPLAY=:99.0
go test -v ./...
```

### Module Management
```bash
# Download dependencies
go mod download

# Update purego to latest
go get -u github.com/ebitengine/purego
go mod tidy
```

## Code Organization

### Package Structure

- **Package name**: `nativeclipboard`
- **Single package**: No subpackages
- **Platform-specific code**: Build tags separate implementations

### API Design

The public API is in `clipboard.go`:
- `Format` type with `Text` and `Image` constants
- `Format.Read()` - Read clipboard data
- `Format.Write([]byte)` - Write clipboard data  
- `Format.Watch(context.Context)` - Monitor clipboard changes

Usage: `nativeclipboard.Text.Read()`, `nativeclipboard.Image.Write(data)`, etc.

All methods return errors. The package initializes automatically via `func init()`.

### Build Tags

Platform-specific files use build constraints:
```go
//go:build darwin && !ios
//go:build (linux || freebsd) && !android
//go:build windows
```

### Linux Implementation (Complete)

Linux clipboard support is split across three files:

**Build tags**: `//go:build linux && !android`

**Files**:
- `clipboard_linux.go` - Routing layer that detects session type and routes to appropriate implementation
- `clipboard_wayland.go` - Wayland implementation using zwlr_data_control protocol
- `clipboard_x11.go` - X11 implementation

The Linux implementation automatically detects and uses the available display server:
1. Checks for Wayland session (WAYLAND_DISPLAY or XDG_SESSION_TYPE=wayland)
2. Tries Wayland first if detected
3. Falls back to X11 if Wayland initialization fails or not available
4. Returns error if neither is available

### X11 Implementation (Linux and FreeBSD - Complete)

**Linux**: Uses clipboard_x11.go with functions: initializeX11, readX11, writeX11, watchX11
**FreeBSD**: Uses clipboard_freebsd.go (dedicated file)

Uses purego to call X11 library functions directly:

1. **Dynamic library loading**:
   - Dynamically loads libX11.so using `purego.Dlopen`
   - Linux paths: libX11.so.6, libX11.so
   - FreeBSD-specific paths: /usr/local/lib, /usr/X11R6/lib
   
2. **X11 API**:
   - `XOpenDisplay` - Connect to X server
   - `XInternAtom` - Get atom identifiers
   - `XSetSelectionOwner` - Claim clipboard ownership
   - `XConvertSelection` - Request clipboard data
   - `XGetWindowProperty` - Read clipboard data
   - `XChangeProperty` - Provide clipboard data

3. **Supported formats**:
   - Text: UTF8_STRING
   - Images: image/png

4. **Requirements**:
   - libX11 must be installed
   - X11 display must be available (DISPLAY environment variable)
   - For headless: use Xvfb virtual framebuffer
   - **CGO_ENABLED=0** - No cgo required (pure Go)

5. **Thread safety**:
   - Use `runtime.LockOSThread()` for thread-sensitive operations

### Wayland Implementation (Linux - Complete)

**Build tags**: `//go:build linux && !android`

**Implementation**: clipboard_wayland.go (dedicated file with pure purego implementation)

Uses the `zwlr_data_control_manager_v1` protocol via purego to interface with libwayland-client:

1. **Pure purego approach**:
   - Dynamically loads libwayland-client.so
   - Uses `purego.NewCallback` for Wayland event handlers
   - No external tools or cgo required
   
2. **Protocol implementation**:
   - Registry callbacks to discover and bind globals (wl_seat, zwlr_data_control_manager_v1)
   - Data control device creation for clipboard access
   - Data offer handling for reading clipboard (with file descriptor transfer)
   - Data source handling for writing clipboard (with send callbacks)
   - Event loop integration using wl_display_dispatch

3. **Key features**:
   - Uses `zwlr_data_control_manager_v1` protocol (wlroots extension)
   - File descriptor-based data transfer using `syscall.Pipe2`
   - Proper callback handling with `purego.NewCallback`
   - Thread-safe operations with `sync.Mutex`
   - Multiple text format offerings for compatibility (text/plain, TEXT, STRING, UTF8_STRING)

4. **Supported formats**:
   - Text: text/plain;charset=utf-8
   - Images: image/png

5. **Current status**:
   - ✅ Full protocol implementation complete
   - ✅ Registry event handling
   - ✅ Read operations via zwlr_data_control_offer_v1
   - ✅ Write operations via zwlr_data_control_source_v1
   - ✅ Watch operations with polling
   - ✅ Automatic fallback to X11 if Wayland unavailable

6. **Requirements**:
   - libwayland-client must be installed
   - WAYLAND_DISPLAY environment variable must be set
   - Compositor must support zwlr_data_control_manager_v1 (most modern compositors do)
   - **CGO_ENABLED=0** - No cgo required (pure Go)

7. **Implementation notes**:
   - Based on the C wl-clipboard implementation
   - Uses purego for FFI without cgo
   - Callback functions use `//go:uintptrescapes` directive
   - Event loop runs in goroutines for write operations
   - Proper resource cleanup and error handling

8. **References**:
   - [wl-clipboard](https://github.com/bugaevc/wl-clipboard) - C reference implementation
   - [wlr-protocols](https://gitlab.freedesktop.org/wlroots/wlr-protocols) - Protocol definitions

### FreeBSD Implementation (Complete)

**Build tags**: `//go:build freebsd`

**Implementation**: clipboard_freebsd.go (dedicated file with X11 support)

Uses the same X11 implementation as Linux, with FreeBSD-specific library paths.

**Note**: FreeBSD only supports X11, not Wayland.

### macOS Implementation (Complete)

Uses purego's objc package to call Objective-C runtime and AppKit framework:

1. **Objective-C integration via purego/objc package**:
   - `objc.GetClass()` - Get class by name
   - `objc.RegisterName()` - Register method selector
   - `objc.ID.Send()` - Call Objective-C methods
   - `objc.Send[T]()` - Call methods with typed return values

2. **Dynamic library loading**:
   - AppKit framework for NSPasteboard
   - objc package auto-loads Objective-C runtime

3. **NSPasteboard API**:
   - `[NSPasteboard generalPasteboard]` - Get clipboard
   - `[pasteboard dataForType:]` - Read data
   - `[pasteboard setData:forType:]` - Write data
   - `[pasteboard changeCount]` - Monitor changes

4. **Key patterns**:
   ```go
   // Get class
   class := objc.GetClass("NSPasteboard")
   
   // Register selector
   sel := objc.RegisterName("generalPasteboard")
   
   // Call method (returns ID)
   result := objc.ID(class).Send(sel)
   
   // Call method with typed return
   count := objc.Send[int64](obj, sel)
   
   // Call method with arguments
   data := obj.Send(sel, arg1, arg2)
   ```

5. **Thread safety**:
   - Use `runtime.LockOSThread()` for thread-sensitive operations
   - Objective-C runtime is thread-safe but pasteboard operations benefit from thread locking

### Import Style

Clean imports, no blank imports:
```go
import (
    "context"
    "github.com/ebitengine/purego"
)
```

## Development Guidelines

### Go Version

- **Required**: Go 1.25.4
- This is a very recent Go version

### Dependency Management

- Dependencies are managed via `go.mod` and `go.sum`
- Use `go get` to add new dependencies, then `go mod tidy`
- Keep `go.mod` and `go.sum` in sync and committed to version control

### File Organization

- Main API in `clipboard.go`
- Platform-specific implementations in `clipboard_<os>.go`
- Tests in `clipboard_test.go` (platform-agnostic where possible)
- Use build tags for platform-specific files

### Code Style

- Follow standard Go conventions
- Use `gofmt` or `go fmt ./...`
- Keep public API minimal and clear
- Document all exported functions and types

## Testing Approach

### Test Organization

- `clipboard_test.go` - Main test file
- Platform-agnostic tests when possible
- Uses standard Go testing package

### Current Tests

1. **TestInit** - Verifies initialization
2. **TestWriteReadText** - Tests text clipboard operations
3. **TestWriteEmptyText** - Tests empty clipboard
4. **TestWatch** - Tests change monitoring
5. **TestMultipleReads** - Tests concurrent reads

### Testing Patterns

```go
// Always initialize first
if err := Init(); err != nil {
    t.Fatalf("Init failed: %v", err)
}

// Add delays for system processing
time.Sleep(100 * time.Millisecond)

// Test with timeouts for async operations
select {
case data := <-ch:
    // process data
case <-time.After(2 * time.Second):
    t.Fatal("Timeout")
}
```

### Platform-Specific Testing

- macOS, Linux, and BSD tests all work with appropriate setup
- Windows tests work natively
- X11-based systems (Linux/BSD) require Xvfb for headless testing
- Add build tags for platform-specific tests if needed

## Platform Considerations

### macOS (Complete)

**Implementation**: Uses Objective-C runtime via purego
- NSPasteboard for clipboard access
- Supports text (NSPasteboardTypeString) and images (NSPasteboardTypePNG)
- Change monitoring via changeCount
- Thread-safe with LockOSThread

**Testing**: Fully tested and working

### Linux and FreeBSD (Complete)

**Implementation**: Unified X11 implementation via purego in `clipboard_x11.go`

**Supported systems**:
- Linux (all distributions with X11)
- FreeBSD (only BSD officially supported by purego)

**Approach**:
- Dynamically loads libX11.so using `purego.Dlopen`
- Searches multiple library paths (Linux and FreeBSD-specific)
- Calls X11 clipboard functions directly
- Supports CLIPBOARD selection (not PRIMARY)
- Handles UTF8_STRING and image/png types

**Requirements**:
- libX11 must be installed
  - Linux: `apt install libx11-dev` or `dnf install libX11-devel`
  - FreeBSD: `pkg install xorg-libraries`
  - OpenBSD: `pkg_add libX11`
  - NetBSD: `pkgin install libX11`
- X11 display must be available (DISPLAY environment variable)
- For headless: use Xvfb virtual framebuffer
- **CGO_ENABLED=0** - No cgo required (pure Go)

**Key X11 functions used**:
- `XOpenDisplay` - Connect to X server
- `XInternAtom` - Get atom identifiers
- `XSetSelectionOwner` - Claim clipboard ownership
- `XConvertSelection` - Request clipboard data
- `XGetWindowProperty` - Read clipboard data
- `XChangeProperty` - Provide clipboard data to requesters

**Implementation notes**:
- Write operation runs in goroutine with event loop
- Clipboard ownership maintained until overwritten
- Uses SelectionRequest/SelectionNotify protocol
- Supports TARGETS atom for format negotiation

**Current state**: Fully implemented, compiles successfully

### Windows (Complete)

**Implementation**: Uses Win32 API via syscall

**Approach**:
- Uses Go's built-in syscall package
- Loads user32.dll and kernel32.dll dynamically
- Supports CF_UNICODETEXT and CF_DIBV5/CF_DIB formats
- Handles UTF-16 encoding for text

**Key Win32 functions used**:
- `OpenClipboard` - Open clipboard for reading/writing
- `CloseClipboard` - Close clipboard
- `EmptyClipboard` - Clear clipboard contents
- `GetClipboardData` - Read clipboard data
- `SetClipboardData` - Write clipboard data
- `GetClipboardSequenceNumber` - Monitor changes
- `GlobalAlloc/GlobalLock/GlobalUnlock` - Memory management

**Format handling**:
- Text: CF_UNICODETEXT with UTF-16 encoding
- Images: CF_DIBV5 (Device Independent Bitmap V5) format
  - Converts PNG ↔ BITMAPV5HEADER + bitmap data
  - Handles fallback to CF_DIB for compatibility

**Implementation notes**:
- Thread safety via `runtime.LockOSThread()`
- Retry loop for OpenClipboard (handles contention)
- Global memory management for clipboard data
- Sequence number monitoring for change detection

**Current state**: Fully implemented, compiles successfully

## Common Patterns

### Objective-C via purego/objc (macOS)

**Get class and selector**:
```go
class := objc.GetClass("NSPasteboard")
sel := objc.RegisterName("generalPasteboard")
```

**Call method (returns ID)**:
```go
result := objc.ID(class).Send(sel)
```

**Call method with typed return**:
```go
count := objc.Send[int64](obj, sel)
length := objc.Send[uint64](data, sel_length)
```

**Call method with arguments**:
```go
data := obj.Send(sel, arg1, arg2)
```

**Load framework and get constants**:
```go
appkit, _ := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
constPtr, _ := purego.Dlsym(appkit, "NSPasteboardTypeString")
constant := objc.ID(*(*uintptr)(unsafe.Pointer(constPtr)))
```

### Thread Safety

```go
runtime.LockOSThread()
defer runtime.UnlockOSThread()
// ... perform thread-sensitive operations
```

### Error Handling

```go
// Check for unavailable clipboard
if data == 0 {
    return nil, ErrUnavailable
}

// Check for unsupported format
switch t {
case FmtText, FmtImage:
    // supported
default:
    return nil, ErrUnsupported
}
```

## Gotchas and Notes

### Blank Imports
The current blank imports (`_ "package"`) are unusual. This typically means:
- The packages have `init()` functions that register handlers
- Or they're being used to ensure certain code is linked into the binary
- Future work should clarify why these are blank imports vs. actual API usage

## Development Workflow

1. **Make changes** to Go files
2. **Build** with `go build ./...` to check for compile errors
3. **Add tests** in `*_test.go` files
4. **Run tests** with `go test ./...`
5. **Update dependencies** if needed: `go get -u <package>` then `go mod tidy`
6. **Format code** with `gofmt` or `go fmt ./...` (standard Go practice)

## Future Considerations

As this codebase grows, consider adding:

- **CI/CD**: GitHub Actions for automated testing
- **Documentation**: README.md with usage examples
- **Examples**: `examples/` directory or `example_test.go` files
- **Benchmarks**: `*_test.go` files with `Benchmark*` functions
- **Linting**: golangci-lint or similar for code quality
- **Platform tests**: Ensure clipboard operations work on all supported platforms

## Additional Resources

- [golang.design/x/clipboard docs](https://pkg.go.dev/golang.design/x/clipboard)
- [ebitengine/purego docs](https://pkg.go.dev/github.com/ebitengine/purego)
- [Go modules documentation](https://go.dev/ref/mod)
- [Effective Go](https://go.dev/doc/effective_go)
