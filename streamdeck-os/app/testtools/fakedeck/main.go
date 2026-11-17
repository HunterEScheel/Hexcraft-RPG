//go:build linux

// fakedeck creates a virtual Stream Deck MK.2 with Linux uhid, so the real
// hidraw driver in streamdeckd can be tested in a VM without hardware.
//
// It decodes every key image the host uploads, answers feature reports, and
// listens on TCP for test commands:
//
//	press N   press and release key N
//	dump      log the current deck face as a base64 PNG
//	stats     print counters
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// linux/uhid.h
const (
	uhidDestroy        = 1
	uhidOutput         = 6
	uhidGetReport      = 9
	uhidGetReportReply = 10
	uhidCreate2        = 11
	uhidInput2         = 12
	uhidSetReport      = 13
	uhidSetReportReply = 14

	uhidDataMax = 4096
	eventSize   = 4 + 128 + 64 + 64 + 2 + 2 + 4 + 4 + 4 + 4 + 4096 // sizeof(struct uhid_event)
	busUSB      = 3
)

// Vendor-defined descriptor with the MK.2's report layout:
// input #1 (511 B), output #2 (1023 B), feature #3 (31 B).
var descriptor = []byte{
	0x06, 0x00, 0xFF, 0x09, 0x01, 0xA1, 0x01,
	0x15, 0x00, 0x26, 0xFF, 0x00, 0x75, 0x08,
	0x85, 0x01, 0x96, 0xFF, 0x01, 0x09, 0x01, 0x81, 0x02,
	0x85, 0x02, 0x96, 0xFF, 0x03, 0x09, 0x02, 0x91, 0x02,
	0x85, 0x03, 0x95, 0x1F, 0x09, 0x03, 0xB1, 0x02,
	0xC0,
}

type fake struct {
	f  *os.File
	mu sync.Mutex

	partial    map[int][]byte
	keys       [15]image.Image
	images     int
	badImages  int
	resets     int
	brightness int
}

func main() {
	listen := flag.String("listen", ":7000", "command port")
	flag.Parse()
	log.SetPrefix("FAKEDECK: ")
	log.SetFlags(0)

	f, err := os.OpenFile("/dev/uhid", os.O_RDWR, 0)
	if err != nil {
		log.Fatal(err)
	}
	d := &fake{f: f, partial: map[int][]byte{}, brightness: -1}

	ev := make([]byte, eventSize)
	binary.LittleEndian.PutUint32(ev[0:], uhidCreate2)
	copy(ev[4:], "Elgato Systems Stream Deck MK.2 (virtual)")
	copy(ev[4+128:], "virtual/streamdeck0")
	copy(ev[4+128+64:], "VIRT0001")
	off := 4 + 128 + 64 + 64
	binary.LittleEndian.PutUint16(ev[off:], uint16(len(descriptor)))
	binary.LittleEndian.PutUint16(ev[off+2:], busUSB)
	binary.LittleEndian.PutUint32(ev[off+4:], 0x0fd9)
	binary.LittleEndian.PutUint32(ev[off+8:], 0x0080)
	binary.LittleEndian.PutUint32(ev[off+12:], 0x0100)
	copy(ev[off+20:], descriptor)
	if _, err := f.Write(ev); err != nil {
		log.Fatalf("UHID_CREATE2: %v", err)
	}
	log.Printf("virtual Stream Deck MK.2 created (0fd9:0080)")

	go d.cmdServer(*listen)
	d.readLoop()
}

func (d *fake) write(ev []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.f.Write(ev); err != nil {
		log.Printf("uhid write: %v", err)
	}
}

func (d *fake) readLoop() {
	buf := make([]byte, eventSize)
	for {
		n, err := d.f.Read(buf)
		if err != nil {
			log.Fatalf("uhid read: %v", err)
		}
		if n < 4 {
			continue
		}
		switch binary.LittleEndian.Uint32(buf) {
		case uhidOutput:
			size := int(binary.LittleEndian.Uint16(buf[4+uhidDataMax:]))
			d.output(append([]byte(nil), buf[4:4+size]...))
		case uhidSetReport:
			id := binary.LittleEndian.Uint32(buf[4:])
			size := int(binary.LittleEndian.Uint16(buf[10:]))
			d.feature(buf[12 : 12+size])
			reply := make([]byte, eventSize)
			binary.LittleEndian.PutUint32(reply[0:], uhidSetReportReply)
			binary.LittleEndian.PutUint32(reply[4:], id)
			d.write(reply)
		case uhidGetReport:
			id := binary.LittleEndian.Uint32(buf[4:])
			rnum := buf[8]
			reply := make([]byte, eventSize)
			binary.LittleEndian.PutUint32(reply[0:], uhidGetReportReply)
			binary.LittleEndian.PutUint32(reply[4:], id)
			binary.LittleEndian.PutUint16(reply[10:], 32)
			reply[12] = rnum
			d.write(reply)
		}
	}
}

func (d *fake) feature(r []byte) {
	if len(r) < 3 || r[0] != 0x03 {
		log.Printf("unexpected feature report % x", r[:min(len(r), 4)])
		return
	}
	switch r[1] {
	case 0x02:
		d.resets++
		log.Printf("reset")
	case 0x08:
		d.brightness = int(r[2])
		log.Printf("brightness %d%%", r[2])
	}
}

func (d *fake) output(r []byte) {
	if len(r) != 1024 || r[0] != 0x02 || r[1] != 0x07 {
		log.Printf("unexpected output report len=%d % x", len(r), r[:min(len(r), 8)])
		return
	}
	key, last := int(r[2]), r[3] == 1
	n := int(r[4]) | int(r[5])<<8
	page := int(r[6]) | int(r[7])<<8
	if key >= 15 || n > 1016 {
		log.Printf("bad image header % x", r[:8])
		return
	}
	if page == 0 {
		d.partial[key] = nil
	}
	d.partial[key] = append(d.partial[key], r[8:8+n]...)
	if !last {
		return
	}
	img, err := jpeg.Decode(bytes.NewReader(d.partial[key]))
	delete(d.partial, key)
	if err != nil || img.Bounds().Dx() != 72 || img.Bounds().Dy() != 72 {
		d.badImages++
		log.Printf("key %d: bad image (%v)", key, err)
		return
	}
	d.mu.Lock()
	d.keys[key] = unflip(img)
	d.images++
	d.mu.Unlock()
}

// unflip undoes the MK.2's 180° mirroring so dumps look like the real deck.
func unflip(src image.Image) image.Image {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			dst.Set(x, y, src.At(b.Max.X-1-x, b.Max.Y-1-y))
		}
	}
	return dst
}

func (d *fake) press(key int) {
	send := func(down bool) {
		ev := make([]byte, eventSize)
		binary.LittleEndian.PutUint32(ev[0:], uhidInput2)
		binary.LittleEndian.PutUint16(ev[4:], 512)
		data := ev[6:]
		data[0] = 0x01
		data[2] = 15
		if down {
			data[4+key] = 1
		}
		d.write(ev)
	}
	send(true)
	time.Sleep(80 * time.Millisecond)
	send(false)
	log.Printf("pressed key %d", key)
}

func (d *fake) dump() string {
	const px, gap = 72, 12
	face := image.NewRGBA(image.Rect(0, 0, 5*px+6*gap, 3*px+4*gap))
	draw.Draw(face, face.Bounds(), &image.Uniform{color.RGBA{0x18, 0x18, 0x18, 0xff}}, image.Point{}, draw.Src)
	d.mu.Lock()
	for k, img := range d.keys {
		if img == nil {
			continue
		}
		x, y := gap+(k%5)*(px+gap), gap+(k/5)*(px+gap)
		draw.Draw(face, image.Rect(x, y, x+px, y+px), img, img.Bounds().Min, draw.Src)
	}
	d.mu.Unlock()
	var buf bytes.Buffer
	_ = png.Encode(&buf, face)
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func (d *fake) stats() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return fmt.Sprintf("images=%d bad=%d resets=%d brightness=%d", d.images, d.badImages, d.resets, d.brightness)
}

func (d *fake) cmdServer(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			sc := bufio.NewScanner(c)
			for sc.Scan() {
				f := strings.Fields(sc.Text())
				if len(f) == 0 {
					continue
				}
				switch f[0] {
				case "press":
					k, err := strconv.Atoi(f[len(f)-1])
					if err != nil || k < 0 || k > 14 {
						fmt.Fprintln(c, "ERR key")
						continue
					}
					d.press(k)
					fmt.Fprintln(c, "OK")
				case "dump":
					fmt.Fprintln(c, d.dump())
				case "stats":
					fmt.Fprintln(c, d.stats())
				default:
					fmt.Fprintln(c, "ERR unknown")
				}
			}
		}(c)
	}
}
