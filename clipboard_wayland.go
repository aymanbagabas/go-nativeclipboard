// Copyright 2025 Ayman Bagabas
// SPDX-License-Identifier: MIT

//go:build linux && !android

package nativeclipboard

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Wayland types
type (
	wlDisplay    uintptr
	wlRegistry   uintptr
	wlSeat       uintptr
	wlProxy      uintptr
	wlEventQueue uintptr
)

// wlr-data-control types
type (
	zwlrDataControlManagerV1 uintptr
	zwlrDataControlDeviceV1  uintptr
	zwlrDataControlOfferV1   uintptr
	zwlrDataControlSourceV1  uintptr
)

// Wayland function pointers
var (
	libwayland uintptr
	
	// Interface pointers exported from libwayland-client
	wlRegistryInterface uintptr

	wlDisplayConnect      func(name *byte) wlDisplay
	wlDisplayDisconnect   func(display wlDisplay)
	wlDisplayRoundtrip    func(display wlDisplay) int32
	wlDisplayDispatch     func(display wlDisplay) int32
	wlDisplayFlush        func(display wlDisplay) int32
	wlProxyMarshalFlags   func(proxy wlProxy, opcode uint32, iface uintptr, version uint32, flags uint32, args ...uintptr) wlProxy
	wlProxyAddListener    func(proxy wlProxy, implementation uintptr, data uintptr) int32
	wlProxyDestroy        func(proxy wlProxy)
	wlProxyGetVersion     func(proxy wlProxy) uint32
	wlProxyMarshal        func(proxy wlProxy, opcode uint32, args ...uintptr)
	wlProxyMarshalConstructor func(proxy wlProxy, opcode uint32, iface uintptr, args ...uintptr) wlProxy
)

// Global Wayland state
type waylandClipboard struct {
	mu                 sync.Mutex
	display            wlDisplay
	registry           wlRegistry
	seat               wlSeat
	dataControlManager zwlrDataControlManagerV1
	dataControlDevice  zwlrDataControlDeviceV1
	currentOffer       zwlrDataControlOfferV1
	currentSource      zwlrDataControlSourceV1
	mimeTypes          []string
	lastData           []byte
	lastUpdate         time.Time
	clipboardData      []byte        // Data to send when compositor requests
	clipboardMimeType  string        // MIME type of clipboard data
	initialized        bool
}

var waylandState waylandClipboard

// Source listener for handling send requests
type sourceListener struct {
	Send      uintptr
	Cancelled uintptr
}

var sourceListenerInstance sourceListener

// Callback functions for source events
//go:uintptrescapes
func sourceHandleSend(data uintptr, source wlProxy, mimeType *byte, fd int32) {
	// This is called when the compositor requests clipboard data
	waylandState.mu.Lock()
	defer waylandState.mu.Unlock()

	if waylandState.clipboardData == nil {
		syscall.Close(int(fd))
		return
	}

	// Write data to the file descriptor
	go func() {
		defer syscall.Close(int(fd))
		
		// Write the clipboard data
		_, err := syscall.Write(int(fd), waylandState.clipboardData)
		if err != nil {
			// Error writing, but we can't do much about it
			return
		}
	}()
}

//go:uintptrescapes
func sourceHandleCancelled(data uintptr, source wlProxy) {
	// This is called when our clipboard data is replaced
	waylandState.mu.Lock()
	defer waylandState.mu.Unlock()
	
	// Clear the current source
	waylandState.currentSource = 0
	waylandState.clipboardData = nil
	waylandState.clipboardMimeType = ""
}

// Registry listener for binding globals
type registryListener struct {
	Global       uintptr
	GlobalRemove uintptr
}

var (
	registryListenerInstance registryListener
	waylandInitOnce          sync.Once
	waylandInitErr           error
)

// Callback functions
//go:uintptrescapes
func registryHandleGlobal(data uintptr, registry wlProxy, name uint32, iface *byte, version uint32) {
	interfaceName := ptrToString(iface)
	
	if interfaceName == "wl_seat" {
		// Bind to seat
		waylandState.mu.Lock()
		waylandState.seat = wlSeat(wlProxyMarshalConstructor(registry, 0, 0, uintptr(name), uintptr(unsafe.Pointer(iface)), uintptr(version)))
		waylandState.mu.Unlock()
	} else if interfaceName == "zwlr_data_control_manager_v1" {
		// Bind to data control manager
		waylandState.mu.Lock()
		waylandState.dataControlManager = zwlrDataControlManagerV1(
			wlProxyMarshalConstructor(registry, 0, 0, uintptr(name), uintptr(unsafe.Pointer(iface)), uintptr(version)),
		)
		waylandState.mu.Unlock()
	}
}

//go:uintptrescapes
func registryHandleGlobalRemove(data uintptr, registry wlProxy, name uint32) {
	// Handle global removal
}

func ptrToString(ptr *byte) string {
	if ptr == nil {
		return ""
	}
	var result []byte
	for i := 0; ; i++ {
		b := *(*byte)(unsafe.Pointer(uintptr(unsafe.Pointer(ptr)) + uintptr(i)))
		if b == 0 {
			break
		}
		result = append(result, b)
	}
	return string(result)
}

func initializeWayland() error {
	waylandInitOnce.Do(func() {
		waylandState.mu.Lock()
		defer waylandState.mu.Unlock()

		// Try to load libwayland-client
		var err error
		libwayland, err = purego.Dlopen("libwayland-client.so.0", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			libwayland, err = purego.Dlopen("libwayland-client.so", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
			if err != nil {
				waylandInitErr = fmt.Errorf("%w: failed to load libwayland-client: %v", ErrUnavailable, err)
				return
			}
		}

		// Load Wayland functions
		purego.RegisterLibFunc(&wlDisplayConnect, libwayland, "wl_display_connect")
		purego.RegisterLibFunc(&wlDisplayDisconnect, libwayland, "wl_display_disconnect")
		purego.RegisterLibFunc(&wlDisplayRoundtrip, libwayland, "wl_display_roundtrip")
		purego.RegisterLibFunc(&wlDisplayDispatch, libwayland, "wl_display_dispatch")
		purego.RegisterLibFunc(&wlDisplayFlush, libwayland, "wl_display_flush")
		purego.RegisterLibFunc(&wlProxyMarshalFlags, libwayland, "wl_proxy_marshal_flags")
		purego.RegisterLibFunc(&wlProxyAddListener, libwayland, "wl_proxy_add_listener")
		purego.RegisterLibFunc(&wlProxyDestroy, libwayland, "wl_proxy_destroy")
		purego.RegisterLibFunc(&wlProxyGetVersion, libwayland, "wl_proxy_get_version")
		purego.RegisterLibFunc(&wlProxyMarshal, libwayland, "wl_proxy_marshal")
		purego.RegisterLibFunc(&wlProxyMarshalConstructor, libwayland, "wl_proxy_marshal_constructor")
		
		// Load interface pointers
		var err2 error
		wlRegistryInterface, err2 = purego.Dlsym(libwayland, "wl_registry_interface")
		if err2 != nil {
			waylandInitErr = fmt.Errorf("%w: failed to load wl_registry_interface: %v", ErrUnavailable, err2)
			return
		}

		// Connect to Wayland display
		waylandState.display = wlDisplayConnect(nil)
		if waylandState.display == 0 {
			waylandInitErr = fmt.Errorf("%w: failed to connect to Wayland display", ErrUnavailable)
			return
		}

		// Setup registry listeners
		registryListenerInstance.Global = purego.NewCallback(registryHandleGlobal)
		registryListenerInstance.GlobalRemove = purego.NewCallback(registryHandleGlobalRemove)

		// Get registry - wl_display_get_registry is opcode 1
		waylandState.registry = wlRegistry(wlProxyMarshalConstructor(
			wlProxy(waylandState.display),
			1, // WL_DISPLAY_GET_REGISTRY opcode
			wlRegistryInterface,
		))
		if waylandState.registry == 0 {
			waylandInitErr = fmt.Errorf("%w: failed to get registry", ErrUnavailable)
			return
		}

		// Add listener to registry
		wlProxyAddListener(
			wlProxy(waylandState.registry),
			uintptr(unsafe.Pointer(&registryListenerInstance)),
			0,
		)

		// Roundtrip to get all globals
		wlDisplayRoundtrip(waylandState.display)

		// Check if we got the required globals
		if waylandState.dataControlManager == 0 {
			waylandInitErr = fmt.Errorf("%w: zwlr_data_control_manager_v1 not available", ErrUnavailable)
			return
		}

		if waylandState.seat == 0 {
			waylandInitErr = fmt.Errorf("%w: wl_seat not available", ErrUnavailable)
			return
		}

		// Create data control device for the seat
		// zwlr_data_control_manager_v1_get_data_device(manager, device_id, seat)
		waylandState.dataControlDevice = zwlrDataControlDeviceV1(
			wlProxyMarshalConstructor(
				wlProxy(waylandState.dataControlManager),
				1, // get_data_device opcode
				0,
				uintptr(unsafe.Pointer(nil)), // new_id
				uintptr(waylandState.seat),
			),
		)

		if waylandState.dataControlDevice == 0 {
			waylandInitErr = fmt.Errorf("%w: failed to create data control device", ErrUnavailable)
			return
		}

		waylandState.initialized = true
		wlDisplayRoundtrip(waylandState.display)
	})

	return waylandInitErr
}

func readWayland(t Format) ([]byte, error) {
	if err := initializeWayland(); err != nil {
		return nil, err
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	waylandState.mu.Lock()
	defer waylandState.mu.Unlock()

	if !waylandState.initialized {
		return nil, ErrUnavailable
	}

	mimeType := mimeTypeForFormat(t)
	if mimeType == "" {
		return nil, ErrUnsupported
	}

	// Create a pipe to receive the data
	fds := make([]int, 2)
	err := syscall.Pipe2(fds, syscall.O_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("failed to create pipe: %w", err)
	}
	defer syscall.Close(fds[0])

	// Request clipboard data
	// zwlr_data_control_offer_v1_receive(offer, mime_type, fd)
	if waylandState.currentOffer != 0 {
		mimeTypeBytes := append([]byte(mimeType), 0)
		wlProxyMarshal(
			wlProxy(waylandState.currentOffer),
			0, // receive opcode
			uintptr(unsafe.Pointer(&mimeTypeBytes[0])),
			uintptr(fds[1]),
		)
		syscall.Close(fds[1])

		// Flush and dispatch to process the request
		wlDisplayFlush(waylandState.display)
		wlDisplayDispatch(waylandState.display)

		// Read data from pipe
		var result []byte
		buf := make([]byte, 4096)
		for {
			n, err := syscall.Read(fds[0], buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, fmt.Errorf("failed to read from pipe: %w", err)
			}
			if n == 0 {
				break
			}
			result = append(result, buf[:n]...)
		}

		return result, nil
	}

	return nil, ErrUnavailable
}

func writeWayland(t Format, buf []byte) (<-chan struct{}, error) {
	if err := initializeWayland(); err != nil {
		return nil, err
	}

	done := make(chan struct{}, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)

		waylandState.mu.Lock()
		
		if !waylandState.initialized {
			waylandState.mu.Unlock()
			return
		}

		mimeType := mimeTypeForFormat(t)
		if mimeType == "" {
			waylandState.mu.Unlock()
			return
		}

		// Store the data for send callbacks
		waylandState.clipboardData = make([]byte, len(buf))
		copy(waylandState.clipboardData, buf)
		waylandState.clipboardMimeType = mimeType

		// Setup source listener callbacks
		if sourceListenerInstance.Send == 0 {
			sourceListenerInstance.Send = purego.NewCallback(sourceHandleSend)
			sourceListenerInstance.Cancelled = purego.NewCallback(sourceHandleCancelled)
		}

		// Create a data source
		// zwlr_data_control_manager_v1_create_data_source(manager, source_id)
		source := zwlrDataControlSourceV1(
			wlProxyMarshalConstructor(
				wlProxy(waylandState.dataControlManager),
				0, // create_data_source opcode
				0,
				uintptr(unsafe.Pointer(nil)), // new_id
			),
		)

		if source == 0 {
			waylandState.mu.Unlock()
			return
		}

		waylandState.currentSource = source

		// Add listener to source for send events
		wlProxyAddListener(
			wlProxy(source),
			uintptr(unsafe.Pointer(&sourceListenerInstance)),
			0,
		)

		// Offer mime type
		// zwlr_data_control_source_v1_offer(source, mime_type)
		mimeTypeBytes := append([]byte(mimeType), 0)
		wlProxyMarshal(
			wlProxy(source),
			0, // offer opcode
			uintptr(unsafe.Pointer(&mimeTypeBytes[0])),
		)

		// Offer additional text formats for compatibility
		if t == Text {
			textFormats := []string{
				"text/plain",
				"TEXT",
				"STRING",
				"UTF8_STRING",
			}
			for _, format := range textFormats {
				formatBytes := append([]byte(format), 0)
				wlProxyMarshal(
					wlProxy(source),
					0, // offer opcode
					uintptr(unsafe.Pointer(&formatBytes[0])),
				)
			}
		}

		// Set selection
		// zwlr_data_control_device_v1_set_selection(device, source)
		wlProxyMarshal(
			wlProxy(waylandState.dataControlDevice),
			0, // set_selection opcode
			uintptr(source),
		)

		waylandState.mu.Unlock()

		// Flush and roundtrip to ensure the selection is set
		wlDisplayFlush(waylandState.display)
		wlDisplayRoundtrip(waylandState.display)

		// Keep dispatching events to handle send requests
		// We'll stay in this loop until the source is cancelled or context is done
		for {
			waylandState.mu.Lock()
			if waylandState.currentSource != source {
				// Source was cancelled
				waylandState.mu.Unlock()
				return
			}
			waylandState.mu.Unlock()

			// Dispatch with timeout
			ret := wlDisplayDispatch(waylandState.display)
			if ret < 0 {
				return
			}
		}
	}()

	return done, nil
}

func watchWayland(ctx context.Context, t Format) <-chan []byte {
	recv := make(chan []byte, 1)

	go func() {
		defer close(recv)

		if err := initializeWayland(); err != nil {
			return
		}

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		var last []byte

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				data, err := readWayland(t)
				if err != nil {
					continue
				}
				if data == nil {
					continue
				}
				if len(last) == 0 || string(last) != string(data) {
					recv <- data
					last = data
				}
			}
		}
	}()

	return recv
}

// Detect if we're running under Wayland or X11
func isWaylandSession() bool {
	// Check for Wayland display socket
	waylandDisplay := os.Getenv("WAYLAND_DISPLAY")
	if waylandDisplay != "" {
		return true
	}

	// Check XDG_SESSION_TYPE
	sessionType := os.Getenv("XDG_SESSION_TYPE")
	if sessionType == "wayland" {
		return true
	}

	return false
}

// Helper to convert Format to MIME type
func waylandMimeType(t Format) string {
	switch t {
	case Text:
		return "text/plain;charset=utf-8"
	case Image:
		return "image/png"
	default:
		return ""
	}
}
