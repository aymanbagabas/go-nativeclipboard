// Copyright 2025 Ayman Bagabas
// SPDX-License-Identifier: MIT

//go:build linux || freebsd

package nativeclipboard

import (
	"bytes"
	"context"
	"fmt"
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
	var err error
	
	// Try common library paths for libX11
	// Linux systems: libX11.so.6, libX11.so
	// FreeBSD often has X11 in /usr/local/lib or /usr/X11R6/lib
	libPaths := []string{
		"libX11.so.6",           // versioned library (Linux, some BSD)
		"libX11.so",             // generic library (Linux, BSD)
		"/usr/local/lib/libX11.so.6",  // FreeBSD, OpenBSD
		"/usr/local/lib/libX11.so",
		"/usr/X11R6/lib/libX11.so.6",  // Older BSD systems
		"/usr/X11R6/lib/libX11.so",
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
	recv := make(chan []byte, 1)
	ticker := time.NewTicker(time.Second)
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
