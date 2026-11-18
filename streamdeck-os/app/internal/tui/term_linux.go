package tui

import (
	"bufio"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// Term is a real terminal (the HDMI console) in raw mode.
type Term struct {
	out  *bufio.Writer
	fd   int
	old  syscall.Termios
	keys chan Key
	in   <-chan []byte
	quit chan struct{}
}

// Open switches the terminal on fd to raw mode. Bytes typed are taken from
// in (the caller owns reading stdin, so line-based prompts and the full-screen
// editor can share it).
func Open(fd int, out *os.File, in <-chan []byte) (*Term, error) {
	t := &Term{out: bufio.NewWriterSize(out, 1<<16), fd: fd, keys: make(chan Key, 64), in: in, quit: make(chan struct{})}
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TCGETS, uintptr(unsafe.Pointer(&t.old))); e != 0 {
		return nil, e
	}
	raw := t.old
	raw.Lflag &^= syscall.ICANON | syscall.ECHO | syscall.ISIG | syscall.IEXTEN
	raw.Iflag &^= syscall.ICRNL | syscall.IXON | syscall.INLCR
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TCSETS, uintptr(unsafe.Pointer(&raw))); e != 0 {
		return nil, e
	}
	t.out.WriteString("\x1b[?25l\x1b[2J") // hide cursor, clear
	t.out.Flush()
	go t.pump()
	return t, nil
}

func (t *Term) pump() {
	var pending []byte
	for {
		select {
		case <-t.quit:
			return
		case chunk, ok := <-t.in:
			if !ok {
				close(t.keys)
				return
			}
			pending = append(pending, chunk...)
			keys, rest := ParseKeys(pending)
			// A lone ESC with nothing after it in this read is the Esc key.
			if len(rest) == 1 && rest[0] == 0x1b {
				keys = append(keys, Key{Name: "esc"})
				rest = nil
			}
			pending = append([]byte(nil), rest...)
			for _, k := range keys {
				select {
				case t.keys <- k:
				case <-t.quit:
					return
				}
			}
		}
	}
}

// Close restores the terminal.
func (t *Term) Close() {
	close(t.quit)
	t.out.WriteString(Reset + "\x1b[2J\x1b[H\x1b[?25h")
	t.out.Flush()
	syscall.Syscall(syscall.SYS_IOCTL, uintptr(t.fd), syscall.TCSETS, uintptr(unsafe.Pointer(&t.old)))
}

func (t *Term) Keys() <-chan Key { return t.keys }

func (t *Term) Size() (int, int) {
	var ws struct{ Row, Col, X, Y uint16 }
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(t.fd), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws))); e != 0 || ws.Col == 0 {
		return 80, 25
	}
	return int(ws.Col), int(ws.Row)
}

// Draw replaces the whole screen with lines.
func (t *Term) Draw(lines []string) {
	w, h := t.Size()
	var b strings.Builder
	b.WriteString("\x1b[H")
	for i := 0; i < h; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		b.WriteString(Reset + Fit(line, w) + Reset)
		if i < h-1 {
			b.WriteString("\r\n")
		}
	}
	t.out.WriteString(b.String())
	t.out.Flush()
}
