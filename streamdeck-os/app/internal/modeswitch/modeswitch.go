// Package modeswitch flips "driver CD" USB Wi-Fi dongles into Wi-Fi mode.
//
// Many cheap Realtek/MediaTek dongles (e.g. generic "AC600 mini" RTL8811CU
// sticks) first show up as a virtual CD-ROM holding Windows drivers (USB ID
// 0bda:1a2b). Windows ejects that CD automatically and the stick re-appears
// as a Wi-Fi adapter (e.g. 0bda:c811). Linux leaves this to the
// usb_modeswitch tool, and since Linux 6.11 usb-storage deliberately ignores
// 0bda:1a2b, so without a switch the dongle just sits there with no driver.
//
// This sends the same "StandardEject" SCSI commands usb_modeswitch uses,
// directly over usbfs.
package modeswitch

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// IDs known to start in driver-CD mode and to switch with StandardEject
// (from the usb_modeswitch device database).
var KnownIDs = map[string]string{
	"0bda:1a2b": "Realtek Wi-Fi (e.g. RTL8811CU/8821CU) in driver-CD mode",
	"0e8d:2870": "MediaTek Wi-Fi (e.g. MT7601U) in driver-CD mode",
}

// usb_modeswitch's StandardEject: ALLOW MEDIUM REMOVAL + START STOP UNIT
// (LoEj) for LUN 0 and LUN 1, as 31-byte mass-storage Command Block Wrappers.
var ejectMsgs = []string{
	"5553424387654321000000000000061e000000000000000000000000000000",
	"5553424397654321000000000000061b000000020000000000000000000000",
	"5553424387654321000000000001061e000000000000000000000000000000",
	"5553424397654321000000000001061b000000020000000000000000000000",
}

// Device is a USB device that should be switched.
type Device struct {
	Dir      string // /sys/bus/usb/devices/1-2
	ID       string
	Bus, Num int
	Name     string
}

var SysUSB = "/sys/bus/usb/devices"

func readTrim(p string) string {
	b, _ := os.ReadFile(p)
	return strings.TrimSpace(string(b))
}

// hasCDROM reports whether the device exposes a CD-ROM (sr*) block device.
func hasCDROM(dir string) bool {
	// /sys/bus/usb/devices entries are symlinks; Walk won't follow the root one.
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	found := false
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() && strings.HasPrefix(info.Name(), "sr") && filepath.Base(filepath.Dir(p)) == "block" {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// Candidates lists devices to switch. extra adds vendor:product IDs that are
// switched only when they show up as a CD-ROM (used by the VM test).
// skipDisk names a block device (e.g. "sr0", the live media) never to touch.
func Candidates(extra []string, skipDisk string) []Device {
	ents, _ := os.ReadDir(SysUSB)
	var out []Device
	for _, e := range ents {
		n := e.Name()
		if strings.Contains(n, ":") || strings.HasPrefix(n, "usb") {
			continue
		}
		dir := filepath.Join(SysUSB, n)
		id := readTrim(filepath.Join(dir, "idVendor")) + ":" + readTrim(filepath.Join(dir, "idProduct"))
		_, known := KnownIDs[id]
		isExtra := false
		for _, x := range extra {
			isExtra = isExtra || strings.EqualFold(x, id)
		}
		if !known && !(isExtra && hasCDROM(dir)) {
			continue
		}
		if skipDisk != "" {
			if m, _ := filepath.Glob(filepath.Join(dir, "*", "host*", "target*", "*", "block", skipDisk)); len(m) > 0 {
				continue
			}
		}
		bus, _ := strconv.Atoi(readTrim(filepath.Join(dir, "busnum")))
		num, _ := strconv.Atoi(readTrim(filepath.Join(dir, "devnum")))
		name := strings.TrimSpace(readTrim(filepath.Join(dir, "manufacturer")) + " " + readTrim(filepath.Join(dir, "product")))
		out = append(out, Device{Dir: dir, ID: id, Bus: bus, Num: num, Name: name})
	}
	return out
}

// Key identifies one plug-in of a device (devnum changes on re-enumeration).
func (d Device) Key() string { return fmt.Sprintf("%d-%d-%s", d.Bus, d.Num, d.ID) }

const (
	usbdevfsBulk             = 0xC0185502 // _IOWR('U', 2, struct usbdevfs_bulktransfer)
	usbdevfsClaimInterface   = 0x8004550F // _IOR('U', 15, unsigned int)
	usbdevfsReleaseInterface = 0x80045510 // _IOR('U', 16, unsigned int)
	usbdevfsIoctl            = 0xC0105512 // _IOWR('U', 18, struct usbdevfs_ioctl)
	usbdevfsDisconnect       = 0x5516     // _IO('U', 22)
)

type bulkTransfer struct {
	Ep      uint32
	Len     uint32
	Timeout uint32
	_       uint32
	Data    uintptr
}

type usbIoctl struct {
	Ifno int32
	Code int32
	Data uintptr
}

func ioctl(fd uintptr, req uintptr, arg unsafe.Pointer) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg)); e != 0 {
		return e
	}
	return nil
}

// endpoints returns the bulk OUT and IN endpoint addresses of an interface.
func endpoints(ifDir string) (out, in uint32, err error) {
	eps, _ := filepath.Glob(filepath.Join(ifDir, "ep_*"))
	for _, ep := range eps {
		if readTrim(filepath.Join(ep, "type")) != "Bulk" {
			continue
		}
		addr, err := strconv.ParseUint(readTrim(filepath.Join(ep, "bEndpointAddress")), 16, 8)
		if err != nil {
			continue
		}
		switch readTrim(filepath.Join(ep, "direction")) {
		case "out":
			if out == 0 {
				out = uint32(addr)
			}
		case "in":
			if in == 0 {
				in = uint32(addr)
			}
		}
	}
	if out == 0 {
		return 0, 0, fmt.Errorf("no bulk OUT endpoint in %s", ifDir)
	}
	return out, in, nil
}

// Eject sends StandardEject to the device's first interface.
func Eject(d Device) error {
	ifDirs, _ := filepath.Glob(filepath.Join(d.Dir, filepath.Base(d.Dir)+":*"))
	if len(ifDirs) == 0 {
		return fmt.Errorf("%s has no interfaces", d.ID)
	}
	ifDir := ifDirs[0]
	ifno, _ := strconv.ParseUint(readTrim(filepath.Join(ifDir, "bInterfaceNumber")), 16, 8)
	epOut, epIn, err := endpoints(ifDir)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(fmt.Sprintf("/dev/bus/usb/%03d/%03d", d.Bus, d.Num), os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	fd := f.Fd()

	// If a kernel driver (usb-storage) holds the interface, detach it.
	if _, err := os.Stat(filepath.Join(ifDir, "driver")); err == nil {
		req := usbIoctl{Ifno: int32(ifno), Code: usbdevfsDisconnect}
		if err := ioctl(fd, usbdevfsIoctl, unsafe.Pointer(&req)); err != nil {
			return fmt.Errorf("detach kernel driver: %w", err)
		}
	}
	n := uint32(ifno)
	if err := ioctl(fd, usbdevfsClaimInterface, unsafe.Pointer(&n)); err != nil {
		return fmt.Errorf("claim interface: %w", err)
	}
	defer ioctl(fd, usbdevfsReleaseInterface, unsafe.Pointer(&n))

	sent := 0
	csw := make([]byte, 13)
	for _, h := range ejectMsgs {
		msg, _ := hex.DecodeString(h)
		bt := bulkTransfer{Ep: epOut, Len: uint32(len(msg)), Timeout: 1000, Data: uintptr(unsafe.Pointer(&msg[0]))}
		err := ioctl(fd, usbdevfsBulk, unsafe.Pointer(&bt))
		runtime.KeepAlive(msg)
		if err != nil {
			break // the device often drops off the bus as soon as it switches
		}
		sent++
		if epIn != 0 { // read the status wrapper; not all devices send one
			rd := bulkTransfer{Ep: epIn, Len: uint32(len(csw)), Timeout: 1000, Data: uintptr(unsafe.Pointer(&csw[0]))}
			_ = ioctl(fd, usbdevfsBulk, unsafe.Pointer(&rd))
			runtime.KeepAlive(csw)
		}
	}
	if sent == 0 {
		return fmt.Errorf("device did not accept the eject command")
	}
	return nil
}
