// Copyright 2025 Ayman Bagabas
// SPDX-License-Identifier: MIT

//go:build windows

package nativeclipboard

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/image/bmp"
)

// Windows clipboard Format constants
const (
	cfUnicodeText = 13
	cfDIB         = 8
	cfDIBV5       = 17
	gmemMoveable  = 0x0002
)

// BITMAPV5HEADER structure
type bitmapV5Header struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
	RedMask       uint32
	GreenMask     uint32
	BlueMask      uint32
	AlphaMask     uint32
	CSType        uint32
	Endpoints     struct {
		CiexyzRed, CiexyzGreen, CiexyzBlue struct {
			CiexyzX, CiexyzY, CiexyzZ int32
		}
	}
	GammaRed    uint32
	GammaGreen  uint32
	GammaBlue   uint32
	Intent      uint32
	ProfileData uint32
	ProfileSize uint32
	Reserved    uint32
}

type bitmapHeader struct {
	Size          uint32
	Width         uint32
	Height        uint32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter uint32
	YPelsPerMeter uint32
	ClrUsed       uint32
	ClrImportant  uint32
}

// Windows API functions
var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	openClipboard                  = user32.NewProc("OpenClipboard")
	closeClipboard                 = user32.NewProc("CloseClipboard")
	emptyClipboard                 = user32.NewProc("EmptyClipboard")
	getClipboardData               = user32.NewProc("GetClipboardData")
	setClipboardData               = user32.NewProc("SetClipboardData")
	isClipboardFormatAvailable     = user32.NewProc("IsClipboardFormatAvailable")
	getClipboardSequenceNumber     = user32.NewProc("GetClipboardSequenceNumber")
	
	gLock   = kernel32.NewProc("GlobalLock")
	gUnlock = kernel32.NewProc("GlobalUnlock")
	gAlloc  = kernel32.NewProc("GlobalAlloc")
	gFree   = kernel32.NewProc("GlobalFree")
	memMove = kernel32.NewProc("RtlMoveMemory")
)

func initialize() error {
	return nil
}

func read(t Format) ([]byte, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var Format uintptr
	switch t {
	case Text:
		Format = cfUnicodeText
	case Image:
		Format = cfDIBV5
	default:
		return nil, ErrUnsupported
	}

	// Check if Format is available
	r, _, _ := isClipboardFormatAvailable.Call(Format)
	if r == 0 {
		return nil, ErrUnavailable
	}

	// Open clipboard
	for {
		r, _, _ := openClipboard.Call(0)
		if r != 0 {
			break
		}
	}
	defer closeClipboard.Call()

	switch t {
	case Text:
		return readText()
	case Image:
		return readImage()
	default:
		return nil, ErrUnsupported
	}
}

func readText() ([]byte, error) {
	hMem, _, _ := getClipboardData.Call(cfUnicodeText)
	if hMem == 0 {
		return nil, ErrUnavailable
	}

	p, _, _ := gLock.Call(hMem)
	if p == 0 {
		return nil, ErrUnavailable
	}
	defer gUnlock.Call(hMem)

	// Find NUL terminator
	n := 0
	for ptr := unsafe.Pointer(p); *(*uint16)(ptr) != 0; n++ {
		ptr = unsafe.Pointer(uintptr(ptr) + unsafe.Sizeof(uint16(0)))
	}

	// Convert UTF-16 to UTF-8
	s := unsafe.Slice((*uint16)(unsafe.Pointer(p)), n)
	return []byte(string(utf16.Decode(s))), nil
}

func readImage() ([]byte, error) {
	hMem, _, _ := getClipboardData.Call(cfDIBV5)
	if hMem == 0 {
		// Try DIB Format
		return readImageDIB()
	}

	p, _, _ := gLock.Call(hMem)
	if p == 0 {
		return nil, ErrUnavailable
	}
	defer gUnlock.Call(hMem)

	info := (*bitmapV5Header)(unsafe.Pointer(p))
	if info.BitCount != 32 {
		return nil, ErrUnsupported
	}

	dataSize := int(info.Size + 4*uint32(info.Width)*uint32(info.Height))
	data := unsafe.Slice((*byte)(unsafe.Pointer(p)), dataSize)

	img := image.NewRGBA(image.Rect(0, 0, int(info.Width), int(info.Height)))
	offset := int(info.Size)
	stride := int(info.Width)

	for y := 0; y < int(info.Height); y++ {
		for x := 0; x < int(info.Width); x++ {
			idx := offset + 4*(y*stride+x)
			xhat := x
			yhat := int(info.Height) - 1 - y
			r := data[idx+2]
			g := data[idx+1]
			b := data[idx+0]
			a := data[idx+3]
			img.SetRGBA(xhat, yhat, color.RGBA{r, g, b, a})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func readImageDIB() ([]byte, error) {
	const (
		fileHeaderLen = 14
		infoHeaderLen = 40
	)

	hClipDat, _, _ := getClipboardData.Call(cfDIB)
	if hClipDat == 0 {
		return nil, ErrUnavailable
	}

	pMemBlk, _, _ := gLock.Call(hClipDat)
	if pMemBlk == 0 {
		return nil, ErrUnavailable
	}
	defer gUnlock.Call(hClipDat)

	bmpHeader := (*bitmapHeader)(unsafe.Pointer(pMemBlk))
	dataSize := bmpHeader.SizeImage + fileHeaderLen + infoHeaderLen

	if bmpHeader.SizeImage == 0 && bmpHeader.Compression == 0 {
		iSizeImage := bmpHeader.Height * ((bmpHeader.Width*uint32(bmpHeader.BitCount)/8 + 3) &^ 3)
		dataSize += iSizeImage
	}

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, uint16('B')|(uint16('M')<<8))
	binary.Write(buf, binary.LittleEndian, uint32(dataSize))
	binary.Write(buf, binary.LittleEndian, uint32(0))
	binary.Write(buf, binary.LittleEndian, uint32(fileHeaderLen+infoHeaderLen))

	headerData := unsafe.Slice((*byte)(unsafe.Pointer(pMemBlk)), dataSize-fileHeaderLen)
	buf.Write(headerData)

	return bmpToPNG(buf)
}

func bmpToPNG(bmpBuf *bytes.Buffer) ([]byte, error) {
	img, err := bmp.Decode(bmpBuf)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func write(t Format, buf []byte) (<-chan struct{}, error) {
	errCh := make(chan error, 1)
	changed := make(chan struct{}, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// Open clipboard
		for {
			r, _, _ := openClipboard.Call(0)
			if r != 0 {
				break
			}
		}

		var err error
		switch t {
		case Text:
			err = writeText(buf)
		case Image:
			err = writeImage(buf)
		default:
			err = ErrUnsupported
		}

		if err != nil {
			closeClipboard.Call()
			errCh <- err
			return
		}

		closeClipboard.Call()

		cnt, _, _ := getClipboardSequenceNumber.Call()
		errCh <- nil

		// Monitor for changes
		for {
			time.Sleep(time.Second)
			cur, _, _ := getClipboardSequenceNumber.Call()
			if cur != cnt {
				changed <- struct{}{}
				close(changed)
				return
			}
		}
	}()

	if err := <-errCh; err != nil {
		return nil, err
	}
	return changed, nil
}

func writeText(buf []byte) error {
	r, _, _ := emptyClipboard.Call()
	if r == 0 {
		return fmt.Errorf("failed to clear clipboard")
	}

	if len(buf) == 0 {
		return nil
	}

	s, err := syscall.UTF16FromString(string(buf))
	if err != nil {
		return fmt.Errorf("failed to convert string: %w", err)
	}

	hMem, _, _ := gAlloc.Call(gmemMoveable, uintptr(len(s)*int(unsafe.Sizeof(s[0]))))
	if hMem == 0 {
		return fmt.Errorf("failed to alloc global memory")
	}

	p, _, _ := gLock.Call(hMem)
	if p == 0 {
		return fmt.Errorf("failed to lock global memory")
	}
	defer gUnlock.Call(hMem)

	memMove.Call(p, uintptr(unsafe.Pointer(&s[0])), uintptr(len(s)*int(unsafe.Sizeof(s[0]))))

	v, _, _ := setClipboardData.Call(cfUnicodeText, hMem)
	if v == 0 {
		gFree.Call(hMem)
		return fmt.Errorf("failed to set clipboard data")
	}

	return nil
}

func writeImage(buf []byte) error {
	r, _, _ := emptyClipboard.Call()
	if r == 0 {
		return fmt.Errorf("failed to clear clipboard")
	}

	if len(buf) == 0 {
		return nil
	}

	img, err := png.Decode(bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("input is not PNG: %w", err)
	}

	offset := unsafe.Sizeof(bitmapV5Header{})
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	imageSize := 4 * width * height

	data := make([]byte, int(offset)+imageSize)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := int(offset) + 4*(y*width+x)
			r, g, b, a := img.At(x, height-1-y).RGBA()
			data[idx+2] = uint8(r >> 8)
			data[idx+1] = uint8(g >> 8)
			data[idx+0] = uint8(b >> 8)
			data[idx+3] = uint8(a >> 8)
		}
	}

	info := bitmapV5Header{}
	info.Size = uint32(offset)
	info.Width = int32(width)
	info.Height = int32(height)
	info.Planes = 1
	info.Compression = 0
	info.SizeImage = uint32(4 * info.Width * info.Height)
	info.RedMask = 0xff0000
	info.GreenMask = 0xff00
	info.BlueMask = 0xff
	info.AlphaMask = 0xff000000
	info.BitCount = 32
	info.CSType = 0x73524742 // sRGB
	info.Intent = 4          // LCS_GM_IMAGES

	infob := (*[unsafe.Sizeof(info)]byte)(unsafe.Pointer(&info))[:]
	copy(data[:], infob)

	hMem, _, _ := gAlloc.Call(gmemMoveable, uintptr(len(data)))
	if hMem == 0 {
		return fmt.Errorf("failed to alloc global memory")
	}

	p, _, _ := gLock.Call(hMem)
	if p == 0 {
		return fmt.Errorf("failed to lock global memory")
	}
	defer gUnlock.Call(hMem)

	memMove.Call(p, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))

	v, _, _ := setClipboardData.Call(cfDIBV5, hMem)
	if v == 0 {
		gFree.Call(hMem)
		return fmt.Errorf("failed to set clipboard data")
	}

	return nil
}

func watch(ctx context.Context, t Format) <-chan []byte {
	recv := make(chan []byte, 1)
	ready := make(chan struct{})

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		cnt, _, _ := getClipboardSequenceNumber.Call()
		ready <- struct{}{}

		for {
			select {
			case <-ctx.Done():
				close(recv)
				return
			case <-ticker.C:
				cur, _, _ := getClipboardSequenceNumber.Call()
				if cnt != cur {
					b, _ := read(t)
					if b != nil {
						recv <- b
					}
					cnt = cur
				}
			}
		}
	}()

	<-ready
	return recv
}
