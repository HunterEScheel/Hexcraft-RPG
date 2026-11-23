package text

import (
	"testing"

	"streamdeckd/internal/config"
)

func TestConsoleColorByHue(t *testing.T) {
	for hex, want := range map[string]int{
		"#3a1d6e": 5, "#b0186e": 5, "#105a6e": 6, "#1db954": 2, "#c0262d": 1, "#e0701b": 3,
		"#1d5fa0": 4, "#202020": 0, "#ffffff": 7, "#6b6b6b": 0, "#e8c020": 3,
	} {
		c, _ := config.ParseColor(hex)
		if got := ConsoleColor(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255); got != want {
			t.Errorf("%s: got %d want %d", hex, got, want)
		}
	}
}
