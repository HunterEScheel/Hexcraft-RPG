package render

import (
	"bytes"
	"image/jpeg"
	"testing"

	"streamdeckd/internal/config"
)

func TestKeySizes(t *testing.T) {
	for _, px := range []int{72, 96, 120} {
		for _, st := range []State{Normal, Busy, OK, Failed} {
			b := config.Button{Label: "Movie Night with a long label", Color: "#3a1d6e", Icon: "missing.png"}
			raw, err := Key(px, b, t.TempDir(), st, true, true)
			if err != nil {
				t.Fatal(err)
			}
			img, err := jpeg.Decode(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Dx() != px || img.Bounds().Dy() != px {
				t.Fatalf("size %v", img.Bounds())
			}
		}
	}
}
