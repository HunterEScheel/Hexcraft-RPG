package ui

import "testing"

func TestHSVRoundTrip(t *testing.T) {
	for _, h := range []string{"#ff0000", "#00ff00", "#0000ff", "#1db954", "#6a2fb0", "#ffffff", "#000000", "#e0701b", "#808080"} {
		if got := HexToHSV(h).Hex(); got != h {
			t.Errorf("%s -> %s", h, got)
		}
	}
	c, ok := HSV{0, 1, 1}.Adjust(Key{Name: "right"})
	if !ok || c.Hex() != "#ff1500" {
		t.Errorf("hue step: %s", c.Hex())
	}
}
