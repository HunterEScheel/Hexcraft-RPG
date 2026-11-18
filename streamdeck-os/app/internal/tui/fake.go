package tui

import (
	"regexp"
	"strings"
	"sync"
)

// Fake is an in-memory Screen for tests.
type Fake struct {
	W, H int
	K    chan Key

	mu   sync.Mutex
	last []string
}

func NewFake(w, h int) *Fake { return &Fake{W: w, H: h, K: make(chan Key, 512)} }

func (f *Fake) Size() (int, int) { return f.W, f.H }
func (f *Fake) Keys() <-chan Key { return f.K }
func (f *Fake) Draw(lines []string) {
	f.mu.Lock()
	f.last = append([]string(nil), lines...)
	f.mu.Unlock()
}

var escRx = regexp.MustCompile("\x1b\\[[0-9;?]*[A-Za-z]")

// Text is the last frame without colour codes.
func (f *Fake) Text() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return escRx.ReplaceAllString(strings.Join(f.last, "\n"), "")
}

// Press queues keys: names like "enter"/"down", or plain text typed letter by letter.
func (f *Fake) Press(keys ...string) {
	for _, k := range keys {
		if csiNamesRev[k] {
			f.K <- Key{Name: k}
			continue
		}
		for _, r := range k {
			f.K <- Key{R: r}
		}
	}
}

var csiNamesRev = map[string]bool{"up": true, "down": true, "left": true, "right": true, "enter": true, "esc": true,
	"backspace": true, "delete": true, "home": true, "end": true, "pgup": true, "pgdn": true, "tab": true}
