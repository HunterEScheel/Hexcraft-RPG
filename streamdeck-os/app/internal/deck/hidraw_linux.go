//go:build linux

package deck

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

// Device is an open Stream Deck.
type Device struct {
	Model Model
	Path  string

	f  *os.File
	mu sync.Mutex // serialises writes
}

// Present reports which supported deck is plugged in ("" if none) without opening it.
func Present() string {
	nodes, _ := filepath.Glob("/sys/class/hidraw/hidraw*")
	for _, n := range nodes {
		raw, err := os.ReadFile(filepath.Join(n, "device", "uevent"))
		if err != nil {
			continue
		}
		if ven, pid, err := parseHIDID(string(raw)); err == nil && ven == VendorElgato {
			if m, err := ModelByPID(pid); err == nil {
				return m.Name
			}
		}
	}
	return ""
}

// Open finds the first supported Stream Deck on /dev/hidraw*.
func Open() (*Device, error) {
	nodes, _ := filepath.Glob("/sys/class/hidraw/hidraw*")
	var lastErr error = ErrNotFound
	for _, n := range nodes {
		raw, err := os.ReadFile(filepath.Join(n, "device", "uevent"))
		if err != nil {
			continue
		}
		ven, pid, err := parseHIDID(string(raw))
		if err != nil || ven != VendorElgato {
			continue
		}
		m, err := ModelByPID(pid)
		if err != nil {
			lastErr = err
			continue
		}
		path := "/dev/" + filepath.Base(n)
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			lastErr = fmt.Errorf("open %s: %w", path, err)
			continue
		}
		return &Device{Model: m, Path: path, f: f}, nil
	}
	return nil, lastErr
}

func (d *Device) Close() error { return d.f.Close() }

// _IOC(_IOC_READ|_IOC_WRITE, 'H', 0x06, len) — HIDIOCSFEATURE
func hidiocsfeature(n int) uintptr {
	const iocRW = 3
	return uintptr(iocRW<<30 | n<<16 | 'H'<<8 | 0x06)
}

func (d *Device) setFeature(report []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	rc, err := d.f.SyscallConn()
	if err != nil {
		return err
	}
	var errno syscall.Errno
	// Control (not Fd) keeps the file in non-blocking mode so Close can interrupt Read.
	if cerr := rc.Control(func(fd uintptr) {
		_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, hidiocsfeature(len(report)),
			uintptr(unsafe.Pointer(&report[0])))
	}); cerr != nil {
		return cerr
	}
	if errno != 0 {
		return fmt.Errorf("HIDIOCSFEATURE: %w", errno)
	}
	return nil
}

func (d *Device) Reset() error                { return d.setFeature(resetReport()) }
func (d *Device) SetBrightness(pct int) error { return d.setFeature(brightnessReport(pct)) }

// SetKeyImage uploads a JPEG (already sized and flipped for the model).
func (d *Device) SetKeyImage(key int, jpeg []byte) error {
	if key < 0 || key >= d.Model.Keys {
		return fmt.Errorf("key %d out of range", key)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, r := range ImageReports(key, jpeg) {
		if _, err := d.f.Write(r); err != nil {
			return err
		}
	}
	return nil
}

// ReadKeys blocks until the next key event and returns all key states.
func (d *Device) ReadKeys() ([]bool, error) {
	buf := make([]byte, 512)
	for {
		n, err := d.f.Read(buf)
		if err != nil {
			if errors.Is(err, os.ErrClosed) {
				return nil, err
			}
			return nil, fmt.Errorf("read %s: %w", d.Path, err)
		}
		if states, ok := ParseKeys(d.Model, buf[:n]); ok {
			return states, nil
		}
	}
}
