package install

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// UEFI boot entries live in efivarfs. Adding one (and putting it first in
// BootOrder) is what efibootmgr does; without it some firmware keeps trying
// old entries (e.g. Windows Boot Manager) or drops to a setup screen.

const (
	efivars        = "/sys/firmware/efi/efivars"
	globalGUID     = "8be4df61-93ca-11d2-aa0d-00e098032b8c"
	attrNV         = 0x1
	attrBS         = 0x2
	attrRT         = 0x4
	loadOptActive  = 0x1
	fsImmutableFl  = 0x10
	fsIocGetFlags  = 0x80086601
	fsIocSetFlags  = 0x40086602
	bootLoaderPath = `\EFI\BOOT\BOOTX64.EFI`
)

// LoadOption builds an EFI_LOAD_OPTION that boots path from a GPT partition.
func LoadOption(desc string, partNum uint32, startLBA, sizeLBA uint64, partGUID [16]byte, path string) []byte {
	// HD(partNum, GPT, guid, start, size)
	hd := make([]byte, 42)
	hd[0], hd[1] = 4, 1
	binary.LittleEndian.PutUint16(hd[2:], 42)
	binary.LittleEndian.PutUint32(hd[4:], partNum)
	binary.LittleEndian.PutUint64(hd[8:], startLBA)
	binary.LittleEndian.PutUint64(hd[16:], sizeLBA)
	copy(hd[24:40], partGUID[:])
	hd[40] = 2 // GPT
	hd[41] = 2 // GUID signature
	// File(path)
	p16 := utf16.Encode([]rune(path + "\x00"))
	file := make([]byte, 4+2*len(p16))
	file[0], file[1] = 4, 4
	binary.LittleEndian.PutUint16(file[2:], uint16(len(file)))
	for i, r := range p16 {
		binary.LittleEndian.PutUint16(file[4+2*i:], r)
	}
	end := []byte{0x7f, 0xff, 4, 0}
	dp := append(append(hd, file...), end...)

	d16 := utf16.Encode([]rune(desc + "\x00"))
	opt := make([]byte, 6+2*len(d16))
	binary.LittleEndian.PutUint32(opt[0:], loadOptActive)
	binary.LittleEndian.PutUint16(opt[4:], uint16(len(dp)))
	for i, r := range d16 {
		binary.LittleEndian.PutUint16(opt[6+2*i:], r)
	}
	return append(opt, dp...)
}

func varPath(name string) string { return filepath.Join(efivars, name+"-"+globalGUID) }

func readVar(name string) ([]byte, error) {
	b, err := os.ReadFile(varPath(name))
	if err != nil {
		return nil, err
	}
	if len(b) < 4 {
		return nil, errors.New("short variable")
	}
	return b[4:], nil
}

func setMutable(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	var flags int32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), fsIocGetFlags, uintptr(unsafe.Pointer(&flags))); e != 0 {
		return
	}
	flags &^= fsImmutableFl
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), fsIocSetFlags, uintptr(unsafe.Pointer(&flags)))
}

func writeVar(name string, data []byte) error {
	p := varPath(name)
	if _, err := os.Stat(p); err == nil {
		setMutable(p)
	}
	buf := make([]byte, 4+len(data))
	binary.LittleEndian.PutUint32(buf, attrNV|attrBS|attrRT)
	copy(buf[4:], data)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	// efivarfs needs the whole variable in a single write.
	if _, err := f.Write(buf); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// AddBootEntry registers StreamDeck OS with the firmware and makes it the
// first boot choice. Returns an error on BIOS (non-UEFI) machines.
func AddBootEntry(layout GPTLayout) error {
	if _, err := os.Stat(efivars); err != nil {
		return fmt.Errorf("not booted with UEFI (no %s)", efivars)
	}
	opt := LoadOption("StreamDeck OS", 1, uint64(layout.FirstLBA), uint64(layout.LastLBA-layout.FirstLBA+1), layout.PartGUID, bootLoaderPath)

	// Reuse our old entry if present, otherwise the first free BootXXXX.
	num := -1
	for i := 0; i < 0x100; i++ {
		b, err := readVar(fmt.Sprintf("Boot%04X", i))
		if err != nil {
			if num < 0 {
				num = i
			}
			continue
		}
		if len(b) > 6 && string(utf16Decode(b[6:])) == "StreamDeck OS" {
			num = i
			break
		}
	}
	if num < 0 {
		return errors.New("no free boot entry")
	}
	if err := writeVar(fmt.Sprintf("Boot%04X", num), opt); err != nil {
		return fmt.Errorf("writing Boot%04X: %w", num, err)
	}
	order := []uint16{uint16(num)}
	if old, err := readVar("BootOrder"); err == nil {
		for i := 0; i+1 < len(old); i += 2 {
			if n := binary.LittleEndian.Uint16(old[i:]); int(n) != num {
				order = append(order, n)
			}
		}
	}
	ob := make([]byte, 2*len(order))
	for i, n := range order {
		binary.LittleEndian.PutUint16(ob[2*i:], n)
	}
	if err := writeVar("BootOrder", ob); err != nil {
		return fmt.Errorf("writing BootOrder: %w", err)
	}
	return nil
}

// utf16Decode reads a NUL-terminated UTF-16LE string.
func utf16Decode(b []byte) []rune {
	var u []uint16
	for i := 0; i+1 < len(b); i += 2 {
		c := binary.LittleEndian.Uint16(b[i:])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return utf16.Decode(u)
}
