package deck

import (
	"bytes"
	"strings"
	"testing"
)

func TestImageReports(t *testing.T) {
	img := make([]byte, 2500)
	for i := range img {
		img[i] = byte(i)
	}
	rs := ImageReports(7, img)
	if len(rs) != 3 {
		t.Fatalf("want 3 reports, got %d", len(rs))
	}
	var got []byte
	for i, r := range rs {
		if len(r) != 1024 || r[0] != 0x02 || r[1] != 0x07 || r[2] != 7 || int(r[6]) != i {
			t.Fatalf("bad header %d: % x", i, r[:8])
		}
		n := int(r[4]) | int(r[5])<<8
		last := r[3] == 1
		if last != (i == 2) {
			t.Fatalf("last flag wrong on %d", i)
		}
		got = append(got, r[8:8+n]...)
	}
	if !bytes.Equal(got, img) {
		t.Fatal("payload mismatch")
	}
	// Exactly one page's worth must not produce an empty trailing report.
	if n := len(ImageReports(0, make([]byte, 1016))); n != 1 {
		t.Fatalf("want 1 report, got %d", n)
	}
}

func TestParseKeys(t *testing.T) {
	m, _ := ModelByPID(0x0080)
	r := make([]byte, 512)
	r[0] = 0x01
	r[4+3] = 1
	st, ok := ParseKeys(m, r)
	if !ok || len(st) != 15 || !st[3] || st[0] {
		t.Fatalf("bad parse: %v %v", st, ok)
	}
	r[0] = 0x02
	if _, ok := ParseKeys(m, r); ok {
		t.Fatal("non-input report must be ignored")
	}
}

func TestParseHIDID(t *testing.T) {
	v, p, err := parseHIDID("DRIVER=hid-generic\nHID_ID=0003:00000FD9:00000080\nHID_NAME=Elgato Systems Stream Deck MK.2\n")
	if err != nil || v != 0x0fd9 || p != 0x0080 {
		t.Fatalf("got %x %x %v", v, p, err)
	}
	if _, err := ModelByPID(0x006c); err == nil || !strings.Contains(err.Error(), "MK.2") {
		t.Fatal("XL should be reported unsupported")
	}
}

func TestReports(t *testing.T) {
	b := brightnessReport(150)
	if len(b) != 32 || b[0] != 3 || b[1] != 8 || b[2] != 100 {
		t.Fatalf("% x", b[:4])
	}
}
