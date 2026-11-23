// Package install puts StreamDeck OS onto a mini PC's internal disk:
// a GPT with one EFI System Partition (FAT32, label STREAMDECK) holding the
// boot loader, kernel, initramfs, button layout and saved settings.
package install

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
)

const (
	Label      = "STREAMDECK"
	espSizeMiB = 1024
)

// Disk is a candidate install target.
type Disk struct {
	Name      string // e.g. nvme0n1
	Model     string
	SizeBytes int64
	Removable bool
}

func (d Disk) Dev() string { return "/dev/" + d.Name }

func (d Disk) String() string {
	gb := float64(d.SizeBytes) / 1e9
	kind := "internal"
	if d.Removable {
		kind = "removable"
	}
	model := d.Model
	if model == "" {
		model = "disk"
	}
	return fmt.Sprintf("%-10s %7.1f GB  %s (%s)", d.Name, gb, model, kind)
}

// PartDev returns the first partition's device node (sda -> sda1, nvme0n1 -> nvme0n1p1).
func (d Disk) PartDev() string {
	if last := d.Name[len(d.Name)-1]; last >= '0' && last <= '9' {
		return "/dev/" + d.Name + "p1"
	}
	return "/dev/" + d.Name + "1"
}

func readSys(p string) string {
	b, _ := os.ReadFile(p)
	return strings.TrimSpace(string(b))
}

// ListDisks returns whole disks that could be installed to, excluding the
// device the live system was booted from (exclude, e.g. "sdb").
func ListDisks(sysBlock, exclude string) []Disk {
	ents, _ := os.ReadDir(sysBlock)
	var out []Disk
	for _, e := range ents {
		n := e.Name()
		if n == exclude || strings.HasPrefix(n, "loop") || strings.HasPrefix(n, "ram") ||
			strings.HasPrefix(n, "sr") || strings.HasPrefix(n, "zram") || strings.HasPrefix(n, "dm-") ||
			strings.HasPrefix(n, "md") || strings.Contains(n, "boot") || strings.Contains(n, "rpmb") {
			continue
		}
		sectors, _ := strconv.ParseInt(readSys(filepath.Join(sysBlock, n, "size")), 10, 64)
		if sectors*512 < 2<<30 { // < 2 GB: not a sensible target
			continue
		}
		out = append(out, Disk{
			Name:      n,
			Model:     strings.TrimSpace(readSys(filepath.Join(sysBlock, n, "device", "model"))),
			SizeBytes: sectors * 512,
			Removable: readSys(filepath.Join(sysBlock, n, "removable")) == "1",
		})
	}
	// Internal disks first.
	sort.SliceStable(out, func(i, j int) bool { return !out[i].Removable && out[j].Removable })
	return out
}

// DiskOf maps a partition/device node (/dev/sdb1, /dev/sr0) to its whole-disk name.
func DiskOf(sysBlock, dev string) string {
	name := filepath.Base(dev)
	if _, err := os.Stat(filepath.Join(sysBlock, name)); err == nil {
		return name
	}
	ents, _ := os.ReadDir(sysBlock)
	for _, e := range ents {
		if _, err := os.Stat(filepath.Join(sysBlock, e.Name(), name)); err == nil {
			return e.Name()
		}
	}
	return ""
}

var espType = guidBytes("C12A7328-F81F-11D2-BA4B-00A0C93EC93B")

// guidBytes encodes a GUID string in GPT's mixed-endian byte order.
func guidBytes(s string) [16]byte {
	h := strings.ReplaceAll(s, "-", "")
	var raw [16]byte
	for i := 0; i < 16; i++ {
		v, _ := strconv.ParseUint(h[2*i:2*i+2], 16, 8)
		raw[i] = byte(v)
	}
	var g [16]byte
	g[0], g[1], g[2], g[3] = raw[3], raw[2], raw[1], raw[0]
	g[4], g[5] = raw[5], raw[4]
	g[6], g[7] = raw[7], raw[6]
	copy(g[8:], raw[8:])
	return g
}

func randomGUID() [16]byte {
	var g [16]byte
	_, _ = rand.Read(g[:])
	g[7] = g[7]&0x0f | 0x40 // version 4 (mixed-endian: byte 7 is the high byte of field 3)
	g[8] = g[8]&0x3f | 0x80
	return g
}

// GPTLayout describes where the ESP lands.
type GPTLayout struct {
	SectorSize int64
	FirstLBA   int64
	LastLBA    int64
	PartGUID   [16]byte
}

// BuildGPT returns the primary area (LBA 0..first usable-1) and the backup
// area (written at the end of the disk) for a disk with one ESP.
func BuildGPT(diskBytes, sectorSize int64, espMiB int64) (primary, backup []byte, backupOffset int64, layout GPTLayout, err error) {
	ss := sectorSize
	totalLBA := diskBytes / ss
	entriesBytes := int64(128 * 128)
	entriesLBAs := (entriesBytes + ss - 1) / ss
	firstUsable := 2 + entriesLBAs
	lastLBA := totalLBA - 1
	lastUsable := lastLBA - entriesLBAs - 1

	align := (1 << 20) / ss
	start := (firstUsable + align - 1) / align * align
	end := start + espMiB*(1<<20)/ss - 1
	if end > lastUsable {
		end = lastUsable
	}
	if end-start < 64*(1<<20)/ss {
		return nil, nil, 0, GPTLayout{}, errors.New("disk too small")
	}

	entries := make([]byte, entriesLBAs*ss)
	e := entries[:128]
	copy(e[0:16], espType[:])
	ug := randomGUID()
	copy(e[16:32], ug[:])
	binary.LittleEndian.PutUint64(e[32:], uint64(start))
	binary.LittleEndian.PutUint64(e[40:], uint64(end))
	for i, r := range utf16.Encode([]rune("EFI System (StreamDeck OS)")) {
		binary.LittleEndian.PutUint16(e[56+2*i:], r)
	}
	entriesCRC := crc32.ChecksumIEEE(entries[:entriesBytes])
	diskGUID := randomGUID()

	header := func(current, other, entriesAt int64) []byte {
		h := make([]byte, ss)
		copy(h[0:8], "EFI PART")
		binary.LittleEndian.PutUint32(h[8:], 0x00010000)
		binary.LittleEndian.PutUint32(h[12:], 92)
		binary.LittleEndian.PutUint64(h[24:], uint64(current))
		binary.LittleEndian.PutUint64(h[32:], uint64(other))
		binary.LittleEndian.PutUint64(h[40:], uint64(firstUsable))
		binary.LittleEndian.PutUint64(h[48:], uint64(lastUsable))
		copy(h[56:72], diskGUID[:])
		binary.LittleEndian.PutUint64(h[72:], uint64(entriesAt))
		binary.LittleEndian.PutUint32(h[80:], 128)
		binary.LittleEndian.PutUint32(h[84:], 128)
		binary.LittleEndian.PutUint32(h[88:], entriesCRC)
		binary.LittleEndian.PutUint32(h[16:], crc32.ChecksumIEEE(h[:92]))
		return h
	}

	// Protective MBR.
	mbr := make([]byte, ss)
	p := mbr[446:]
	p[4] = 0xEE
	binary.LittleEndian.PutUint32(p[8:], 1)
	size := totalLBA - 1
	if size > 0xFFFFFFFF {
		size = 0xFFFFFFFF
	}
	binary.LittleEndian.PutUint32(p[12:], uint32(size))
	p[2] = 0x02 // CHS start 0/0/2
	p[5], p[6], p[7] = 0xFF, 0xFF, 0xFF
	mbr[510], mbr[511] = 0x55, 0xAA

	primary = append(append(mbr, header(1, lastLBA, 2)...), entries...)
	backupEntriesLBA := lastLBA - entriesLBAs
	backup = append(append([]byte{}, entries...), header(lastLBA, 1, backupEntriesLBA)...)
	return primary, backup, backupEntriesLBA * ss, GPTLayout{SectorSize: ss, FirstLBA: start, LastLBA: end, PartGUID: ug}, nil
}

// Progress receives human-readable steps.
type Progress func(format string, a ...any)

// Install wipes disk, creates the ESP and copies the system onto it.
//
// payloadDir holds vmlinuz, initrd*.img and BOOTX64.EFI (from the live media);
// layoutDir is copied to /streamdeck (config.json, icons, govee-devices.txt);
// stateDir is copied to /streamdeck/state (secrets, Wi-Fi).
func Install(d Disk, payloadDir, layoutDir, stateDir string, say Progress) error {
	// Check everything we need BEFORE touching the disk.
	if err := CheckPayload(payloadDir); err != nil {
		return err
	}
	ss, _ := strconv.ParseInt(readSys("/sys/block/"+d.Name+"/queue/logical_block_size"), 10, 64)
	if ss == 0 {
		ss = 512
	}
	f, err := os.OpenFile(d.Dev(), os.O_RDWR|os.O_SYNC, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	say("Erasing old partition tables...")
	zero := make([]byte, 1<<20)
	if _, err := f.WriteAt(zero, 0); err != nil {
		return err
	}
	if _, err := f.WriteAt(zero, d.SizeBytes-int64(len(zero))); err != nil {
		return err
	}
	primary, backup, backupOff, layout, err := BuildGPT(d.SizeBytes, ss, espSizeMiB)
	if err != nil {
		return err
	}
	say("Writing partition table...")
	if _, err := f.WriteAt(primary, 0); err != nil {
		return err
	}
	if _, err := f.WriteAt(backup, backupOff); err != nil {
		return err
	}
	// Wipe the start of the new partition so no stale filesystem is detected.
	if _, err := f.WriteAt(zero, layout.FirstLBA*ss); err != nil {
		return err
	}
	_ = f.Sync()
	const blkrrpart = 0x125F
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), blkrrpart, 0); errno != 0 {
		return fmt.Errorf("re-reading partition table: %w", errno)
	}
	part := d.PartDev()
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(part); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	say("Formatting %s (FAT32)...", part)
	if out, err := exec.Command("mkdosfs", "-n", Label, part).CombinedOutput(); err != nil {
		return fmt.Errorf("mkdosfs: %v: %s", err, out)
	}
	mnt, err := os.MkdirTemp("", "esp")
	if err != nil {
		return err
	}
	if err := syscall.Mount(part, mnt, "vfat", 0, ""); err != nil {
		return fmt.Errorf("mount %s: %w", part, err)
	}
	defer func() {
		syscall.Sync()
		_ = syscall.Unmount(mnt, 0)
	}()

	say("Copying system files...")
	boot := filepath.Join(mnt, "streamdeck", "boot")
	var initrds []string
	ents, err := os.ReadDir(payloadDir)
	if err != nil {
		return err
	}
	for _, e := range ents {
		n := e.Name()
		switch {
		case n == "BOOTX64.EFI":
			if err := copyFile(filepath.Join(payloadDir, n), filepath.Join(mnt, "EFI", "BOOT", "BOOTX64.EFI")); err != nil {
				return err
			}
		case n == "vmlinuz" || strings.HasSuffix(n, ".img"):
			if err := copyFile(filepath.Join(payloadDir, n), filepath.Join(boot, n)); err != nil {
				return err
			}
			if strings.HasSuffix(n, ".img") {
				initrds = append(initrds, "/streamdeck/boot/"+n)
			}
		}
	}
	sort.Strings(initrds) // initrd.img before test.img
	cfg := "set timeout=0\nmenuentry \"StreamDeck OS\" {\n" +
		"\tlinux /streamdeck/boot/vmlinuz " + KernelArgs + "\n" +
		"\tinitrd " + strings.Join(initrds, " ") + "\n}\n"
	if err := os.WriteFile(filepath.Join(boot, "installed.cfg"), []byte(cfg), 0o644); err != nil {
		return err
	}
	if err := copyTree(layoutDir, filepath.Join(mnt, "streamdeck")); err != nil {
		return err
	}
	if err := copyTree(stateDir, filepath.Join(mnt, "streamdeck", "state")); err != nil {
		return err
	}
	if err := AddBootEntry(layout); err != nil {
		// Not fatal: firmware falls back to \EFI\BOOT\BOOTX64.EFI.
		say("Note: could not add a UEFI boot entry (%v)", err)
	} else {
		say("Added \"StreamDeck OS\" as the first UEFI boot option.")
	}
	say("Done.")
	return nil
}

// PayloadFiles must all be on the live media for an install.
var PayloadFiles = []string{"vmlinuz", "initrd.img", "BOOTX64.EFI"}

// CheckPayload reports which installer files are missing, if any.
func CheckPayload(dir string) error {
	var missing []string
	for _, f := range PayloadFiles {
		if st, err := os.Stat(filepath.Join(dir, f)); err != nil || st.Size() == 0 {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("installer files not found in %s (missing %s)", dir, strings.Join(missing, ", "))
	}
	return nil
}

// MediaDisk turns the /run/streamdeck-media entry ("/dev/sdb", "/dev/sdb1" or
// "/dev/sdb1:streamdeck-os.iso" for an ISO file on the stick) into the disk name.
func MediaDisk(sysBlock, media string) string {
	dev, _, _ := strings.Cut(strings.TrimSpace(media), ":")
	if dev == "" {
		return ""
	}
	return DiskOf(sysBlock, dev)
}

// KernelArgs for the installed system.
const KernelArgs = "quiet loglevel=3 fbcon=font:TER16x32"

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyTree(src, dst string) error {
	if src == "" {
		return nil
	}
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(p, filepath.Join(dst, rel))
	})
}

// FindInstalled returns the partition of an existing StreamDeck OS install
// (FAT label STREAMDECK with our boot files), or "".
func FindInstalled() string {
	out, err := exec.Command("findfs", "LABEL="+Label).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Update replaces the system files of an existing install, keeping the
// button layout and settings (streamdeck/config.json, streamdeck/state).
func Update(part, payloadDir string, say Progress) error {
	if err := CheckPayload(payloadDir); err != nil {
		return err
	}
	mnt, err := os.MkdirTemp("", "esp")
	if err != nil {
		return err
	}
	if err := syscall.Mount(part, mnt, "vfat", 0, ""); err != nil {
		return fmt.Errorf("mount %s: %w", part, err)
	}
	defer func() {
		syscall.Sync()
		_ = syscall.Unmount(mnt, 0)
	}()
	if _, err := os.Stat(filepath.Join(mnt, "streamdeck", "boot", "installed.cfg")); err != nil {
		return fmt.Errorf("%s doesn't look like a StreamDeck OS install", part)
	}
	say("Updating system files on %s (your settings and buttons are kept)...", part)
	boot := filepath.Join(mnt, "streamdeck", "boot")
	var initrds []string
	ents, err := os.ReadDir(payloadDir)
	if err != nil {
		return err
	}
	// Remove old initramfs parts first so a removed one doesn't linger.
	old, _ := filepath.Glob(filepath.Join(boot, "*.img"))
	for _, o := range old {
		os.Remove(o)
	}
	for _, e := range ents {
		n := e.Name()
		switch {
		case n == "BOOTX64.EFI":
			if err := copyFile(filepath.Join(payloadDir, n), filepath.Join(mnt, "EFI", "BOOT", "BOOTX64.EFI")); err != nil {
				return err
			}
		case n == "vmlinuz" || strings.HasSuffix(n, ".img"):
			if err := copyFile(filepath.Join(payloadDir, n), filepath.Join(boot, n)); err != nil {
				return err
			}
			if strings.HasSuffix(n, ".img") {
				initrds = append(initrds, "/streamdeck/boot/"+n)
			}
		}
	}
	sort.Strings(initrds)
	cfg := "set timeout=0\nmenuentry \"StreamDeck OS\" {\n" +
		"\tlinux /streamdeck/boot/vmlinuz " + KernelArgs + "\n" +
		"\tinitrd " + strings.Join(initrds, " ") + "\n}\n"
	if err := os.WriteFile(filepath.Join(boot, "installed.cfg"), []byte(cfg), 0o644); err != nil {
		return err
	}
	// Firmware can forget boot entries (e.g. after booting a USB stick or a
	// settings reset): register StreamDeck OS again.
	if l, err := layoutOf(part); err == nil {
		if err := AddBootEntry(l); err != nil {
			say("Note: could not refresh the UEFI boot entry (%v)", err)
		}
	}
	say("Done.")
	return nil
}

// layoutOf reads partition 1's position and GUID from the disk's GPT.
func layoutOf(part string) (GPTLayout, error) {
	disk := DiskOf("/sys/block", part)
	if disk == "" {
		return GPTLayout{}, fmt.Errorf("no disk for %s", part)
	}
	ss, _ := strconv.ParseInt(readSys("/sys/block/"+disk+"/queue/logical_block_size"), 10, 64)
	if ss == 0 {
		ss = 512
	}
	f, err := os.Open("/dev/" + disk)
	if err != nil {
		return GPTLayout{}, err
	}
	defer f.Close()
	return readGPT(f, ss)
}

// readGPT parses the primary GPT header and first partition entry.
func readGPT(r io.ReaderAt, ss int64) (GPTLayout, error) {
	h := make([]byte, 92)
	if _, err := r.ReadAt(h, ss); err != nil {
		return GPTLayout{}, err
	}
	if string(h[0:8]) != "EFI PART" {
		return GPTLayout{}, errors.New("no GPT")
	}
	entriesLBA := int64(binary.LittleEndian.Uint64(h[72:]))
	e := make([]byte, 128)
	if _, err := r.ReadAt(e, entriesLBA*ss); err != nil {
		return GPTLayout{}, err
	}
	var l GPTLayout
	l.SectorSize = ss
	copy(l.PartGUID[:], e[16:32])
	l.FirstLBA = int64(binary.LittleEndian.Uint64(e[32:]))
	l.LastLBA = int64(binary.LittleEndian.Uint64(e[40:]))
	return l, nil
}
