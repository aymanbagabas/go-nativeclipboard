// Copyright 2025 Ayman Bagabas
// SPDX-License-Identifier: MIT

//go:build linux && !android

package nativeclipboard

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// X11 types
type (
	Display uintptr
	Window  uintptr
	Atom    uintptr
	Time    uintptr
	Bool    int
)

// X11 constants
const (
	None         = 0
	CurrentTime  = 0
	AnyPropertyType = 0
	PropModeReplace = 0
	Success         = 0
	SelectionNotify = 31
	SelectionClear  = 29
	SelectionRequest = 30
)

// XEvent is a union in C, we need the largest variant
type XEvent struct {
	typ int32
	pad [23]uintptr // Ensure it's large enough for all event types
}

type XSelectionEvent struct {
	typ       int32
	_         [3]byte // padding
	serial    uintptr
	send_event Bool
	display   Display
	requestor Window
	selection Atom
	target    Atom
	property  Atom
	time      Time
}

type XSelectionRequestEvent struct {
	typ       int32
	_         [3]byte
	serial    uintptr
	send_event Bool
	display   Display
	owner     Window
	requestor Window
	selection Atom
	target    Atom
	property  Atom
	time      Time
}

// X11 function pointers
var (
	libX11 uintptr

	xOpenDisplay        func(display_name uintptr) Display
	xCloseDisplay       func(display Display)
	xDefaultRootWindow  func(display Display) Window
	xCreateSimpleWindow func(display Display, parent Window, x, y int, width, height, border_width uint, border, background uintptr) Window
	xInternAtom         func(display Display, atom_name string, only_if_exists Bool) Atom
	xSetSelectionOwner  func(display Display, selection Atom, owner Window, time Time)
	xGetSelectionOwner  func(display Display, selection Atom) Window
	xNextEvent          func(display Display, event *XEvent)
	xChangeProperty     func(display Display, w Window, property Atom, typ Atom, Format int, mode int, data *byte, nelements int) int
	xSendEvent          func(display Display, w Window, propagate Bool, event_mask int64, event *XEvent)
	xGetWindowProperty  func(display Display, w Window, property Atom, long_offset, long_length int64, delete Bool, req_type Atom, actual_type_return *Atom, actual_format_return *int, nitems_return *uint64, bytes_after_return *uint64, prop_return **byte) int
	xFree               func(data unsafe.Pointer)
	xDeleteProperty     func(display Display, w Window, property Atom)
	xConvertSelection   func(display Display, selection Atom, target Atom, property Atom, requestor Window, time Time)
)

var helpmsg = `%w: Failed to initialize the X11 display, and the clipboard package
will not work properly. Install the following dependency may help:

	# Debian/Ubuntu
	apt install -y libx11-dev

	# Fedora/RHEL
	dnf install -y libX11-devel

	# FreeBSD
	pkg install xorg-libraries

If the clipboard package is in an environment without a frame buffer,
such as a cloud server, it may also be necessary to install xvfb and
initialize a virtual frame buffer:

	Xvfb :99 -screen 0 1024x768x24 > /dev/null 2>&1 &
	export DISPLAY=:99.0

Then this package should be ready to use.
`

func initialize() error {
	// Try Wayland first if WAYLAND_DISPLAY is set
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		err := initializeWayland()
		if err == nil {
			useWayland = true
			return nil
		}
		// Wayland failed, fall through to X11
	}

	// Try X11 as fallback or if DISPLAY is set
	var err error
	
	// Try common library paths for libX11
	// Linux systems: libX11.so.6, libX11.so
	libPaths := []string{
		"libX11.so.6",           // versioned library (Linux)
		"libX11.so",             // generic library (Linux)
	}
	
	for _, path := range libPaths {
		libX11, err = purego.Dlopen(path, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err == nil {
			break
		}
	}
	
	if err != nil {
		return fmt.Errorf(helpmsg, ErrUnavailable)
	}

	// Load all X11 functions
	purego.RegisterLibFunc(&xOpenDisplay, libX11, "XOpenDisplay")
	purego.RegisterLibFunc(&xCloseDisplay, libX11, "XCloseDisplay")
	purego.RegisterLibFunc(&xDefaultRootWindow, libX11, "XDefaultRootWindow")
	purego.RegisterLibFunc(&xCreateSimpleWindow, libX11, "XCreateSimpleWindow")
	purego.RegisterLibFunc(&xInternAtom, libX11, "XInternAtom")
	purego.RegisterLibFunc(&xSetSelectionOwner, libX11, "XSetSelectionOwner")
	purego.RegisterLibFunc(&xGetSelectionOwner, libX11, "XGetSelectionOwner")
	purego.RegisterLibFunc(&xNextEvent, libX11, "XNextEvent")
	purego.RegisterLibFunc(&xChangeProperty, libX11, "XChangeProperty")
	purego.RegisterLibFunc(&xSendEvent, libX11, "XSendEvent")
	purego.RegisterLibFunc(&xGetWindowProperty, libX11, "XGetWindowProperty")
	purego.RegisterLibFunc(&xFree, libX11, "XFree")
	purego.RegisterLibFunc(&xDeleteProperty, libX11, "XDeleteProperty")
	purego.RegisterLibFunc(&xConvertSelection, libX11, "XConvertSelection")

	// Test if we can open display
	var display Display
	for i := 0; i < 42; i++ {
		display = xOpenDisplay(0)
		if display != 0 {
			break
		}
	}
	if display == 0 {
		return fmt.Errorf(helpmsg, ErrUnavailable)
	}
	xCloseDisplay(display)

	return nil
}

func read(t Format) ([]byte, error) {
	// Check if Wayland is being used
	if useWayland {
		return readWayland(t)
	}

	// Use X11 implementation
	var atomType string
	switch t {
	case Text:
		atomType = "UTF8_STRING"
	case Image:
		atomType = "image/png"
	default:
		return nil, ErrUnsupported
	}

	return readX11(atomType)
}

func readX11(atomType string) ([]byte, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var display Display
	for i := 0; i < 42; i++ {
		display = xOpenDisplay(0)
		if display != 0 {
			break
		}
	}
	if display == 0 {
		return nil, ErrUnavailable
	}
	defer xCloseDisplay(display)

	root := xDefaultRootWindow(display)
	window := xCreateSimpleWindow(display, root, 0, 0, 1, 1, 0, 0, 0)

	sel := xInternAtom(display, "CLIPBOARD", 0)
	prop := xInternAtom(display, "GOLANG_DESIGN_DATA", 0)
	target := xInternAtom(display, atomType, 1)

	if target == None {
		return nil, ErrUnsupported
	}

	xConvertSelection(display, sel, target, prop, window, CurrentTime)

	var event XEvent
	for {
		xNextEvent(display, &event)
		if event.typ != SelectionNotify {
			continue
		}
		break
	}

	// Cast event to XSelectionEvent
	sev := (*XSelectionEvent)(unsafe.Pointer(&event))

	if sev.property == None || sev.selection != sel || sev.property != prop {
		return nil, ErrUnavailable
	}

	var actual Atom
	var Format int
	var nitems, bytesAfter uint64
	var data *byte

	ret := xGetWindowProperty(sev.display, sev.requestor, sev.property,
		0, ^int64(0), 0, AnyPropertyType,
		&actual, &Format, &nitems, &bytesAfter, &data)

	if ret != Success || data == nil {
		return nil, ErrUnavailable
	}
	defer xFree(unsafe.Pointer(data))

	if nitems == 0 {
		return nil, nil
	}

	// Copy data to Go slice
	result := make([]byte, nitems)
	dataSlice := unsafe.Slice(data, nitems)
	copy(result, dataSlice)

	xDeleteProperty(sev.display, sev.requestor, sev.property)

	return result, nil
}

func write(t Format, buf []byte) (<-chan struct{}, error) {
	// Check if Wayland is being used
	if useWayland {
		return writeWayland(t, buf)
	}

	// Use X11 implementation
	var atomType string
	switch t {
	case Text:
		atomType = "UTF8_STRING"
	case Image:
		atomType = "image/png"
	default:
		return nil, ErrUnsupported
	}

	done := make(chan struct{}, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		var display Display
		for i := 0; i < 42; i++ {
			display = xOpenDisplay(0)
			if display != 0 {
				break
			}
		}
		if display == 0 {
			close(done)
			return
		}
		defer xCloseDisplay(display)

		root := xDefaultRootWindow(display)
		window := xCreateSimpleWindow(display, root, 0, 0, 1, 1, 0, 0, 0)

		sel := xInternAtom(display, "CLIPBOARD", 0)
		atomString := xInternAtom(display, "UTF8_STRING", 0)
		atomImage := xInternAtom(display, "image/png", 0)
		targetsAtom := xInternAtom(display, "TARGETS", 0)
		xaAtom := xInternAtom(display, "ATOM", 0)

		target := xInternAtom(display, atomType, 1)
		if target == None {
			close(done)
			return
		}

		xSetSelectionOwner(display, sel, window, CurrentTime)
		if xGetSelectionOwner(display, sel) != window {
			close(done)
			return
		}

		var event XEvent
		for {
			xNextEvent(display, &event)

			switch event.typ {
			case SelectionClear:
				close(done)
				return

			case SelectionRequest:
				req := (*XSelectionRequestEvent)(unsafe.Pointer(&event))
				if req.selection != sel {
					continue
				}

				var selEvent XSelectionEvent
				selEvent.typ = SelectionNotify
				selEvent.display = req.display
				selEvent.requestor = req.requestor
				selEvent.selection = req.selection
				selEvent.target = req.target
				selEvent.property = req.property
				selEvent.time = req.time

				if req.target == atomString && target == atomString {
					if len(buf) > 0 {
						xChangeProperty(selEvent.display, selEvent.requestor, selEvent.property,
							atomString, 8, PropModeReplace, &buf[0], len(buf))
					}
				} else if req.target == atomImage && target == atomImage {
					if len(buf) > 0 {
						xChangeProperty(selEvent.display, selEvent.requestor, selEvent.property,
							atomImage, 8, PropModeReplace, &buf[0], len(buf))
					}
				} else if req.target == targetsAtom {
					targets := []Atom{atomString, atomImage}
					xChangeProperty(selEvent.display, selEvent.requestor, selEvent.property,
						xaAtom, 32, PropModeReplace, (*byte)(unsafe.Pointer(&targets[0])), len(targets))
				} else {
					selEvent.property = None
				}

				xSendEvent(selEvent.display, selEvent.requestor, 0, 0, (*XEvent)(unsafe.Pointer(&selEvent)))
			}
		}
	}()

	return done, nil
}

func watch(ctx context.Context, t Format) <-chan []byte {
	// Check if Wayland is being used
	if useWayland {
		return watchWayland(ctx, t)
	}

	// Use X11 implementation
	recv := make(chan []byte, 1)
	ticker := time.NewTicker(clipboardPollInterval)
	last, _ := read(t)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				close(recv)
				return
			case <-ticker.C:
				b, _ := read(t)
				if b == nil {
					continue
				}
				if !bytes.Equal(last, b) {
					recv <- b
					last = b
				}
			}
		}
	}()

	return recv
}

// Wayland support
// Implementation of Wayland clipboard using the wl_data_device_manager protocol

var useWayland = false

// Wayland types
type (
	wl_display              uintptr
	wl_registry             uintptr
	wl_compositor           uintptr
	wl_seat                 uintptr
	wl_data_device_manager  uintptr
	wl_data_device          uintptr
	wl_data_source          uintptr
	wl_data_offer           uintptr
	wl_event_queue          uintptr
	wl_proxy                uintptr
)

// Wayland function pointers
var (
	libwayland uintptr

	wl_display_connect          func(name *byte) wl_display
	wl_display_disconnect       func(display wl_display)
	wl_display_get_registry     func(display wl_display) wl_registry
	wl_display_roundtrip        func(display wl_display) int32
	wl_display_dispatch         func(display wl_display) int32
	wl_display_dispatch_pending func(display wl_display) int32
	wl_display_flush            func(display wl_display) int32
	wl_display_sync             func(display wl_display) uintptr
	
	// Note: These use ...interface{} for variadic args to match C's variadic functions.
	// In a complete implementation, these would need careful handling of argument types
	// to match the expected Wayland protocol message formats.
	wl_proxy_marshal            func(proxy wl_proxy, opcode uint32, args ...interface{}) wl_proxy
	wl_proxy_marshal_constructor func(proxy wl_proxy, opcode uint32, iface uintptr, args ...interface{}) wl_proxy
	wl_proxy_marshal_constructor_versioned func(proxy wl_proxy, opcode uint32, iface uintptr, version uint32, args ...interface{}) wl_proxy
	wl_proxy_add_listener       func(proxy wl_proxy, implementation uintptr, data uintptr) int32
	wl_proxy_destroy            func(proxy wl_proxy)
	wl_proxy_get_user_data      func(proxy wl_proxy) uintptr
	wl_proxy_set_user_data      func(proxy wl_proxy, user_data uintptr)
)

// Wayland global state
type waylandClipboard struct {
	display           wl_display
	registry          wl_registry
	compositor        wl_compositor
	seat              wl_seat
	dataDeviceManager wl_data_device_manager
	dataDevice        wl_data_device
	serialNumber      uint32
	initialized       bool
	useExternalTools  bool  // If true, use wl-copy/wl-paste instead of direct implementation
}

var waylandState waylandClipboard

var waylandHelpmsg = `Failed to initialize Wayland clipboard. Install libwayland-client:

	# Debian/Ubuntu
	apt install -y libwayland-client0

	# Fedora/RHEL
	dnf install -y wayland-devel

	# Arch Linux
	pacman -S wayland

Make sure WAYLAND_DISPLAY environment variable is set.
`

// Wayland protocol opcodes
const (
	WL_REGISTRY_BIND                          = 0
	WL_DATA_DEVICE_MANAGER_CREATE_DATA_SOURCE = 0
	WL_DATA_DEVICE_MANAGER_GET_DATA_DEVICE    = 1
	WL_DATA_SOURCE_OFFER                      = 0
	WL_DATA_SOURCE_DESTROY                    = 1
	WL_DATA_DEVICE_SET_SELECTION              = 1
)

// MIME types for clipboard
const (
	mimeTypeTextPlain     = "text/plain"
	mimeTypeTextPlainUtf8 = "text/plain;charset=utf-8"
	mimeTypeImagePNG      = "image/png"
)

// Clipboard polling interval for watch operations
const clipboardPollInterval = time.Second

// getMimeType returns the MIME type string for a given clipboard Format
func getMimeType(t Format) (string, error) {
	switch t {
	case Text:
		return mimeTypeTextPlainUtf8, nil
	case Image:
		return mimeTypeImagePNG, nil
	default:
		return "", ErrUnsupported
	}
}

// initializeWayland attempts to initialize Wayland clipboard support
func initializeWayland() error {
	// Check if WAYLAND_DISPLAY is set
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		return fmt.Errorf("WAYLAND_DISPLAY not set")
	}

	// Check if wl-copy and wl-paste are available
	// These are from wl-clipboard package and provide a reliable way to interact with Wayland clipboard
	if _, err := exec.LookPath("wl-copy"); err == nil {
		if _, err := exec.LookPath("wl-paste"); err == nil {
			// Use external tools approach - this is reliable and tested
			waylandState.useExternalTools = true
			waylandState.initialized = true
			return nil
		}
	}

	// If external tools aren't available, try direct libwayland approach
	// This is more complex and requires proper callback handling
	
	// Try to load libwayland-client.so
	var err error
	libPaths := []string{
		"libwayland-client.so.0",
		"libwayland-client.so",
		"/usr/lib/libwayland-client.so.0",
		"/usr/lib/x86_64-linux-gnu/libwayland-client.so.0",
		"/usr/lib64/libwayland-client.so.0",
		"/usr/lib/aarch64-linux-gnu/libwayland-client.so.0",
	}

	for _, path := range libPaths {
		libwayland, err = purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			break
		}
	}

	if err != nil {
		return fmt.Errorf("%s\nError: %w", waylandHelpmsg, err)
	}

	// Load Wayland functions
	purego.RegisterLibFunc(&wl_display_connect, libwayland, "wl_display_connect")
	purego.RegisterLibFunc(&wl_display_disconnect, libwayland, "wl_display_disconnect")
	purego.RegisterLibFunc(&wl_display_get_registry, libwayland, "wl_display_get_registry")
	purego.RegisterLibFunc(&wl_display_roundtrip, libwayland, "wl_display_roundtrip")
	purego.RegisterLibFunc(&wl_display_dispatch, libwayland, "wl_display_dispatch")
	purego.RegisterLibFunc(&wl_display_dispatch_pending, libwayland, "wl_display_dispatch_pending")
	purego.RegisterLibFunc(&wl_display_flush, libwayland, "wl_display_flush")
	purego.RegisterLibFunc(&wl_display_sync, libwayland, "wl_display_sync")

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Connect to Wayland display
	waylandState.display = wl_display_connect(nil)
	if waylandState.display == 0 {
		return fmt.Errorf("failed to connect to Wayland display")
	}

	// Test the connection works
	if wl_display_roundtrip(waylandState.display) < 0 {
		wl_display_disconnect(waylandState.display)
		return fmt.Errorf("failed to communicate with Wayland display")
	}

	// Note: Full implementation of Wayland clipboard requires:
	// 1. Implementing C callbacks using purego.NewCallback for registry events
	// 2. Handling the wl_data_device_manager protocol
	// 3. Managing file descriptor-based data transfer
	// 
	// This is complex with purego due to:
	// - Callback trampolines need careful lifetime management
	// - Event loops require integration with Wayland's fd-based dispatch
	// - Interface matching needs proper struct layouts
	//
	// For now, we require wl-copy/wl-paste tools for full functionality
	// Clean up and return error to fall back to X11

	wl_display_disconnect(waylandState.display)
	waylandState.display = 0

	return fmt.Errorf("Wayland clipboard requires wl-copy/wl-paste tools (install wl-clipboard package) - falling back to X11")
}

// Wayland clipboard read implementation using wl-paste
func readWayland(t Format) ([]byte, error) {
	if !waylandState.initialized {
		return nil, ErrUnavailable
	}

	if !waylandState.useExternalTools {
		return nil, fmt.Errorf("Wayland clipboard requires wl-copy/wl-paste tools")
	}

	// Get MIME type for the requested format
	mimeType, err := getMimeType(t)
	if err != nil {
		return nil, err
	}

	// Use wl-paste to read from clipboard
	cmd := exec.Command("wl-paste", "-t", mimeType)
	
	output, err := cmd.Output()
	if err != nil {
		// Check if it's just that clipboard is empty
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				// Clipboard is empty or doesn't have this MIME type
				return nil, nil
			}
		}
		return nil, fmt.Errorf("wl-paste failed: %w", err)
	}

	return output, nil
}

// Wayland clipboard write implementation using wl-copy
func writeWayland(t Format, buf []byte) (<-chan struct{}, error) {
	if !waylandState.initialized {
		return nil, ErrUnavailable
	}

	if !waylandState.useExternalTools {
		return nil, fmt.Errorf("Wayland clipboard requires wl-copy/wl-paste tools")
	}

	// Get MIME type for the requested format
	mimeType, err := getMimeType(t)
	if err != nil {
		return nil, err
	}

	// Use wl-copy to write to clipboard
	cmd := exec.Command("wl-copy", "-t", mimeType)
	
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start wl-copy: %w", err)
	}

	// Write data to stdin synchronously to avoid race with cmd.Wait()
	_, err = io.Copy(stdin, bytes.NewReader(buf))
	stdin.Close()
	
	if err != nil {
		cmd.Wait() // Clean up the process
		return nil, fmt.Errorf("failed to write to wl-copy: %w", err)
	}

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("wl-copy failed: %w", err)
	}

	// Return a closed channel to indicate write completion
	// Note: This channel does NOT signal when the clipboard is overwritten by another app.
	// To monitor clipboard changes, use the Watch() function instead.
	changed := make(chan struct{})
	close(changed)

	return changed, nil
}

// Wayland clipboard watch implementation
func watchWayland(ctx context.Context, t Format) <-chan []byte {
	ch := make(chan []byte, 1)

	if !waylandState.initialized {
		close(ch)
		return ch
	}

	go func() {
		defer close(ch)

		// Use consistent polling interval across all implementations
		ticker := time.NewTicker(clipboardPollInterval)
		defer ticker.Stop()

		var lastData []byte
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				data, err := readWayland(t)
				if err != nil {
					continue
				}

				if !bytes.Equal(data, lastData) {
					lastData = data
					select {
					case ch <- data:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return ch
}
