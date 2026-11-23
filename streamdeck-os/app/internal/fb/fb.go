// Package fb draws to the Linux framebuffer (/dev/fb0), switches the console
// into graphics mode so the text console doesn't paint over it, and reads
// mice/touchpads from /dev/input/mice.
package fb

import (
	"errors"
	"fmt"
	"image"
	"os"
	"syscall"
	"unsafe"
)

// Device is an open framebuffer.
type Device struct {
	f      *os.File
	mem    []byte
	W, H   int
	stride int
	bpp    int
	// Bit offsets of each colour in a pixel.
	rOff, gOff, bOff uint32
	rLen, gLen, bLen uint32
}

const (
	fbioGetVScreenInfo = 0x4600
	fbioGetFScreenInfo = 0x4602
)

type bitfield struct{ Offset, Length, MsbRight uint32 }

type varScreenInfo struct {
	XRes, YRes, XResVirtual, YResVirtual, XOffset, YOffset, BitsPerPixel, Grayscale uint32
	Red, Green, Blue, Transp                                                        bitfield
	NonStd, Activate, Height, Width, AccelFlags, PixClock, LeftMargin, RightMargin  uint32
	UpperMargin, LowerMargin, HSyncLen, VSyncLen, Sync, Vmode, Rotate, Colorspace   uint32
	Reserved                                                                        [4]uint32
}

type fixScreenInfo struct {
	ID                             [16]byte
	SmemStart                      uintptr
	SmemLen, Type, TypeAux, Visual uint32
	XPanStep, YPanStep, YWrapStep  uint16
	LineLength                     uint32
	MmioStart                      uintptr
	MmioLen, Accel                 uint32
	Capabilities                   uint16
	Reserved                       [2]uint16
}

// Open maps /dev/fb0.
func Open(path string) (*Device, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	var v varScreenInfo
	var fx fixScreenInfo
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), fbioGetVScreenInfo, uintptr(unsafe.Pointer(&v))); e != 0 {
		f.Close()
		return nil, fmt.Errorf("FBIOGET_VSCREENINFO: %w", e)
	}
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), fbioGetFScreenInfo, uintptr(unsafe.Pointer(&fx))); e != 0 {
		f.Close()
		return nil, fmt.Errorf("FBIOGET_FSCREENINFO: %w", e)
	}
	if v.BitsPerPixel != 32 && v.BitsPerPixel != 16 {
		f.Close()
		return nil, fmt.Errorf("unsupported framebuffer: %d bits per pixel", v.BitsPerPixel)
	}
	size := int(fx.LineLength) * int(v.YRes)
	mem, err := syscall.Mmap(int(f.Fd()), 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("mmap framebuffer: %w", err)
	}
	return &Device{f: f, mem: mem, W: int(v.XRes), H: int(v.YRes), stride: int(fx.LineLength), bpp: int(v.BitsPerPixel),
		rOff: v.Red.Offset, gOff: v.Green.Offset, bOff: v.Blue.Offset,
		rLen: v.Red.Length, gLen: v.Green.Length, bLen: v.Blue.Length}, nil
}

func (d *Device) Close() {
	syscall.Munmap(d.mem)
	d.f.Close()
}

// Flush copies rect r of img (same size as the screen) to the framebuffer.
func (d *Device) Flush(img *image.RGBA, r image.Rectangle) {
	r = r.Intersect(image.Rect(0, 0, d.W, d.H))
	if r.Empty() {
		return
	}
	if d.bpp == 32 && d.rOff == 16 && d.gOff == 8 && d.bOff == 0 {
		// Common XRGB8888 fast path.
		for y := r.Min.Y; y < r.Max.Y; y++ {
			src := img.Pix[img.PixOffset(r.Min.X, y):img.PixOffset(r.Max.X, y)]
			dst := d.mem[y*d.stride+r.Min.X*4 : y*d.stride+r.Max.X*4]
			for i := 0; i+3 < len(src); i += 4 {
				dst[i], dst[i+1], dst[i+2], dst[i+3] = src[i+2], src[i+1], src[i], 0xff
			}
		}
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			o := img.PixOffset(x, y)
			rr, gg, bb := uint32(img.Pix[o]), uint32(img.Pix[o+1]), uint32(img.Pix[o+2])
			px := (rr>>(8-d.rLen))<<d.rOff | (gg>>(8-d.gLen))<<d.gOff | (bb>>(8-d.bLen))<<d.bOff
			if d.bpp == 32 {
				i := y*d.stride + x*4
				d.mem[i], d.mem[i+1], d.mem[i+2], d.mem[i+3] = byte(px), byte(px>>8), byte(px>>16), byte(px>>24)
			} else {
				i := y*d.stride + x*2
				d.mem[i], d.mem[i+1] = byte(px), byte(px>>8)
			}
		}
	}
}

// ---- console graphics mode ----

const (
	kdSetMode  = 0x4B3A
	kdGraphics = 1
	kdText     = 0
)

// GraphicsMode stops the text console drawing on the screen (fd is the tty).
// The returned function switches back.
func GraphicsMode(fd uintptr) (restore func(), err error) {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, kdSetMode, kdGraphics); e != 0 {
		return func() {}, e
	}
	return func() { syscall.Syscall(syscall.SYS_IOCTL, fd, kdSetMode, kdText) }, nil
}

// ---- mice and touchpads ----

// MouseEvent is relative motion, buttons held and wheel steps.
type MouseEvent struct {
	DX, DY  int
	Buttons int // bit 0 left, 1 right, 2 middle
	Wheel   int // + = up
}

// Mice reads /dev/input/mice (all mice and touchpads together), with the
// scroll wheel enabled. Returns nil if there is no such device.
func Mice(path string) (<-chan MouseEvent, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	// IntelliMouse handshake: set sample rate 200, 100, 80, then ask the ID.
	// /dev/input/mice answers 3 when it will send 4-byte packets with a wheel.
	_, _ = f.Write([]byte{0xf3, 200, 0xf3, 100, 0xf3, 80, 0xf2})
	ack := make([]byte, 64)
	wheel := false
	if n, err := f.Read(ack); err == nil {
		for i := 0; i < n; i++ {
			if ack[i] == 3 {
				wheel = true
			}
		}
	}
	size := 3
	if wheel {
		size = 4
	}
	ch := make(chan MouseEvent, 64)
	go func() {
		defer f.Close()
		buf := make([]byte, size)
		for {
			if _, err := readFull(f, buf); err != nil {
				close(ch)
				return
			}
			b := buf[0]
			if b&0x08 == 0 { // lost sync: skip a byte
				continue
			}
			dx, dy := int(buf[1]), int(buf[2])
			if b&0x10 != 0 {
				dx -= 256
			}
			if b&0x20 != 0 {
				dy -= 256
			}
			ev := MouseEvent{DX: dx, DY: -dy, Buttons: int(b & 7)}
			if wheel {
				w := int(int8(buf[3]<<4) >> 4)
				ev.Wheel = -w
			}
			select {
			case ch <- ev:
			default:
			}
		}
	}()
	return ch, nil
}

func readFull(f *os.File, b []byte) (int, error) {
	n := 0
	for n < len(b) {
		m, err := f.Read(b[n:])
		if err != nil {
			return n, err
		}
		if m == 0 {
			return n, errors.New("eof")
		}
		n += m
	}
	return n, nil
}

// Memory is an off-screen XRGB8888 framebuffer (tests and screenshots).
func Memory(w, h int) *Device {
	return &Device{mem: make([]byte, w*h*4), W: w, H: h, stride: w * 4, bpp: 32, rOff: 16, gOff: 8, bOff: 0, rLen: 8, gLen: 8, bLen: 8}
}

// Snapshot copies the screen contents into an image.
func (d *Device) Snapshot() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, d.W, d.H))
	for y := 0; y < d.H; y++ {
		for x := 0; x < d.W; x++ {
			i, o := y*d.stride+x*4, img.PixOffset(x, y)
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = d.mem[i+2], d.mem[i+1], d.mem[i], 0xff
		}
	}
	return img
}
