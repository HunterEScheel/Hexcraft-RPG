//go:build linux

package deck

import "testing"

func TestIoctlNumber(t *testing.T) {
	// HIDIOCSFEATURE(32) as computed by the C macro on Linux.
	if got := hidiocsfeature(32); got != 0xC0204806 {
		t.Fatalf("got %#x", got)
	}
}
