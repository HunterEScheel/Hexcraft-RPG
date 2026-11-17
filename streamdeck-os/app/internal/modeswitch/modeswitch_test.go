package modeswitch

import (
	"os"
	"path/filepath"
	"testing"
)

func dev(t *testing.T, root, name, ven, prod string, cd string) {
	d := filepath.Join(root, "devices", name)
	os.MkdirAll(filepath.Join(d, name+":1.0"), 0o755)
	for f, v := range map[string]string{"idVendor": ven, "idProduct": prod, "busnum": "2", "devnum": "5", "product": "DISK"} {
		os.WriteFile(filepath.Join(d, f), []byte(v+"\n"), 0o644)
	}
	if cd != "" {
		os.MkdirAll(filepath.Join(d, name+":1.0", "host7", "target7:0:0", "7:0:0:0", "block", cd), 0o755)
	}
	os.Symlink(d, filepath.Join(SysUSB, name))
}

func TestCandidates(t *testing.T) {
	root := t.TempDir()
	SysUSB = filepath.Join(root, "bus")
	os.MkdirAll(SysUSB, 0o755)
	dev(t, root, "2-1", "0bda", "1a2b", "")    // Realtek driver-CD mode, ignored by usb-storage
	dev(t, root, "2-2", "0bda", "c811", "")    // already in Wi-Fi mode
	dev(t, root, "2-3", "46f4", "0001", "sda") // test ID but a disk (e.g. the boot stick)
	dev(t, root, "2-4", "46f4", "0001", "sr1") // test ID as CD-ROM
	dev(t, root, "2-5", "46f4", "0001", "sr0") // the live media itself

	got := map[string]bool{}
	for _, d := range Candidates([]string{"46f4:0001"}, "sr0") {
		got[filepath.Base(d.Dir)] = true
	}
	if len(got) != 2 || !got["2-1"] || !got["2-4"] {
		t.Fatalf("got %v", got)
	}
	if c := Candidates(nil, ""); len(c) != 1 || c[0].ID != "0bda:1a2b" || c[0].Key() != "2-5-0bda:1a2b" {
		t.Fatalf("production list: %+v", c)
	}
}
