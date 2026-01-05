// Copyright 2025 Ayman Bagabas
// SPDX-License-Identifier: MIT

//go:build darwin

package nativeclipboard

import (
	"context"
	"runtime"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	// Classes
	nsPasteboardClass objc.Class
	nsDataClass       objc.Class

	// Selectors
	sel_generalPasteboard    objc.SEL
	sel_dataForType          objc.SEL
	sel_clearContents        objc.SEL
	sel_setData_forType      objc.SEL
	sel_changeCount          objc.SEL
	sel_dataWithBytes_length objc.SEL
	sel_bytes                objc.SEL
	sel_length               objc.SEL

	// Pasteboard types (NSString constants)
	NSPasteboardTypeString objc.ID
	NSPasteboardTypePNG    objc.ID
)

func initialize() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Load AppKit framework for NSPasteboard
	appkit, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}

	// Get classes
	nsPasteboardClass = objc.GetClass("NSPasteboard")
	nsDataClass = objc.GetClass("NSData")

	// Register selectors
	sel_generalPasteboard = objc.RegisterName("generalPasteboard")
	sel_dataForType = objc.RegisterName("dataForType:")
	sel_clearContents = objc.RegisterName("clearContents")
	sel_setData_forType = objc.RegisterName("setData:forType:")
	sel_changeCount = objc.RegisterName("changeCount")
	sel_dataWithBytes_length = objc.RegisterName("dataWithBytes:length:")
	sel_bytes = objc.RegisterName("bytes")
	sel_length = objc.RegisterName("length")

	// Get pasteboard type constants from AppKit
	// NSPasteboardTypeString and NSPasteboardTypePNG are NSString constants
	typeStringPtr, err := purego.Dlsym(appkit, "NSPasteboardTypeString")
	if err != nil {
		return err
	}
	typePNGPtr, err := purego.Dlsym(appkit, "NSPasteboardTypePNG")
	if err != nil {
		return err
	}

	// Dereference the pointers to get the actual NSString objects
	NSPasteboardTypeString = objc.ID(*(*uintptr)(unsafe.Pointer(typeStringPtr)))
	NSPasteboardTypePNG = objc.ID(*(*uintptr)(unsafe.Pointer(typePNGPtr)))

	return nil
}

func read(t Format) ([]byte, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Get general pasteboard: [NSPasteboard generalPasteboard]
	pasteboard := objc.ID(nsPasteboardClass).Send(sel_generalPasteboard)
	if pasteboard == 0 {
		return nil, ErrUnavailable
	}

	// Determine the pasteboard type
	var pasteboardType objc.ID
	switch t {
	case Text:
		pasteboardType = NSPasteboardTypeString
	case Image:
		pasteboardType = NSPasteboardTypePNG
	default:
		return nil, ErrUnsupported
	}

	// Get data: [pasteboard dataForType:type]
	data := pasteboard.Send(sel_dataForType, pasteboardType)
	if data == 0 {
		return nil, ErrUnavailable
	}

	// Get length: [data length]
	length := objc.Send[uint64](data, sel_length)
	if length == 0 {
		return nil, nil
	}

	// Get bytes: [data bytes]
	bytes := data.Send(sel_bytes)
	if bytes == 0 {
		return nil, ErrUnavailable
	}

	// Copy data to Go slice
	result := make([]byte, length)
	copyBytes(result, uintptr(bytes), int(length))

	return result, nil
}

func write(t Format, buf []byte) (<-chan struct{}, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Get general pasteboard
	pasteboard := objc.ID(nsPasteboardClass).Send(sel_generalPasteboard)
	if pasteboard == 0 {
		return nil, ErrUnavailable
	}

	// Clear contents: [pasteboard clearContents]
	pasteboard.Send(sel_clearContents)

	if len(buf) == 0 {
		return nil, nil
	}

	// Create NSData from bytes: [NSData dataWithBytes:buf length:len(buf)]
	var data objc.ID
	if len(buf) > 0 {
		data = objc.ID(nsDataClass).Send(sel_dataWithBytes_length, unsafe.Pointer(&buf[0]), uint64(len(buf)))
	} else {
		data = objc.ID(nsDataClass).Send(sel_dataWithBytes_length, unsafe.Pointer(nil), uint64(0))
	}

	if data == 0 {
		return nil, ErrUnavailable
	}

	// Determine the pasteboard type
	var pasteboardType objc.ID
	switch t {
	case Text:
		pasteboardType = NSPasteboardTypeString
	case Image:
		pasteboardType = NSPasteboardTypePNG
	default:
		return nil, ErrUnsupported
	}

	// Set data: [pasteboard setData:data forType:type]
	ok := objc.Send[bool](pasteboard, sel_setData_forType, data, pasteboardType)
	if !ok {
		return nil, ErrUnavailable
	}

	// Monitor for changes
	changed := make(chan struct{}, 1)
	initialCount := objc.Send[int64](pasteboard, sel_changeCount)

	go func() {
		for {
			time.Sleep(time.Second)
			pb := objc.ID(nsPasteboardClass).Send(sel_generalPasteboard)
			if pb == 0 {
				continue
			}
			count := objc.Send[int64](pb, sel_changeCount)
			if count != initialCount {
				changed <- struct{}{}
				close(changed)
				return
			}
		}
	}()

	return changed, nil
}

func watch(ctx context.Context, t Format) <-chan []byte {
	recv := make(chan []byte, 1)
	ticker := time.NewTicker(time.Second)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		pasteboard := objc.ID(nsPasteboardClass).Send(sel_generalPasteboard)
		lastCount := objc.Send[int64](pasteboard, sel_changeCount)

		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				close(recv)
				return
			case <-ticker.C:
				pb := objc.ID(nsPasteboardClass).Send(sel_generalPasteboard)
				currentCount := objc.Send[int64](pb, sel_changeCount)
				if currentCount != lastCount {
					b, _ := read(t)
					if b != nil {
						recv <- b
					}
					lastCount = currentCount
				}
			}
		}
	}()

	return recv
}

func copyBytes(dst []byte, src uintptr, length int) {
	if length == 0 {
		return
	}
	// Use unsafe to copy from C memory to Go slice
	srcSlice := unsafe.Slice((*byte)(unsafe.Pointer(src)), length)
	copy(dst, srcSlice)
}
