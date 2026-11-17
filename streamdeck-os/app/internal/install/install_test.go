package install

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestGPTAcceptedBySfdisk(t *testing.T) {
	sf, err := exec.LookPath("sfdisk")
	if err != nil {
		t.Skip("sfdisk not installed")
	}
	for _, size := range []int64{8 << 30, 3 << 30, 512 << 30} {
		img := t.TempDir() + "/disk.img"
		f, err := os.Create(img)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(size); err != nil {
			t.Fatal(err)
		}
		primary, backup, off, layout, err := BuildGPT(size, 512, espSizeMiB)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteAt(primary, 0)
		f.WriteAt(backup, off)
		f.Close()

		out, err := exec.Command(sf, "--verify", img).CombinedOutput()
		if err != nil || strings.Contains(strings.ToLower(string(out)), "corrupt") {
			t.Fatalf("verify %d: %v\n%s", size, err, out)
		}
		out, err = exec.Command(sf, "--dump", img).CombinedOutput()
		if err != nil {
			t.Fatalf("dump: %v\n%s", err, out)
		}
		d := string(out)
		if !strings.Contains(d, "label: gpt") || !strings.Contains(d, "type=C12A7328-F81F-11D2-BA4B-00A0C93EC93B") ||
			!strings.Contains(d, "start=        2048") || !strings.Contains(d, "size=     2097152") {
			t.Fatalf("unexpected layout for %d:\n%s", size, d)
		}
		if sg, err := exec.LookPath("sgdisk"); err == nil {
			out, _ := exec.Command(sg, "-v", img).CombinedOutput()
			if !strings.Contains(string(out), "No problems found") {
				t.Fatalf("sgdisk -v %d:\n%s", size, out)
			}
		}
		if layout.FirstLBA != 2048 {
			t.Fatalf("first LBA %d", layout.FirstLBA)
		}
	}
}

func TestPartDev(t *testing.T) {
	for name, want := range map[string]string{"sda": "/dev/sda1", "nvme0n1": "/dev/nvme0n1p1", "mmcblk0": "/dev/mmcblk0p1"} {
		if got := (Disk{Name: name}).PartDev(); got != want {
			t.Errorf("%s: %s", name, got)
		}
	}
}

func TestLoadOption(t *testing.T) {
	g := guidBytes("07A14772-8D3A-4911-A9DA-160D0EF03372")
	b := LoadOption("StreamDeck OS", 1, 2048, 2097152, g, `\EFI\BOOT\BOOTX64.EFI`)
	// attrs(4) + dplen(2) + "StreamDeck OS\0" (28) + HD(42) + File(4+44) + End(4)
	if len(b) != 4+2+28+42+48+4 {
		t.Fatalf("len %d", len(b))
	}
	if got := string(utf16Decode(b[6:])); got != "StreamDeck OS" {
		t.Fatalf("desc %q", got)
	}
	hd := b[34:]
	if hd[0] != 4 || hd[1] != 1 || hd[40] != 2 || hd[41] != 2 {
		t.Fatalf("bad HD node % x", hd[:4])
	}
	end := b[len(b)-4:]
	if end[0] != 0x7f || end[1] != 0xff {
		t.Fatalf("bad end node % x", end)
	}
}
