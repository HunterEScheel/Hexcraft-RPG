// Package deck drives an Elgato Stream Deck MK.2 directly over Linux hidraw
// (no libusb/hidapi needed).
//
// MK.2 protocol (same as the "Original V2" generation):
//   - input report 0x01: key states start at byte 4, one byte per key
//   - output report 0x02, 1024 bytes: 8-byte header + JPEG chunk
//   - feature report 0x03, 32 bytes: 0x02 = reset, 0x08 = brightness
//   - key images are 72x72 JPEG, mirrored horizontally and vertically
package deck

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const VendorElgato = 0x0fd9

// Model describes the Stream Deck hardware.
type Model struct {
	Name       string
	ProductID  uint16
	Keys       int
	Cols, Rows int
	// Key image size in pixels (square).
	Pixels int
	// Images must be mirrored before upload.
	FlipH, FlipV bool
}

// MK2 is the Stream Deck MK.2 (15 keys, 5x3).
var MK2 = Model{Name: "Stream Deck MK.2", ProductID: 0x0080, Keys: 15, Cols: 5, Rows: 3, Pixels: 72, FlipH: true, FlipV: true}

// Models lists supported product IDs. 0x00a5 is the later MK.2 revision
// with scissor switches; it speaks the identical protocol.
var Models = []Model{
	MK2,
	{Name: "Stream Deck MK.2 (scissor keys)", ProductID: 0x00a5, Keys: 15, Cols: 5, Rows: 3, Pixels: 72, FlipH: true, FlipV: true},
}

var unsupported = map[uint16]string{
	0x0060: "Stream Deck Original (V1)",
	0x006d: "Stream Deck Original V2",
	0x0063: "Stream Deck Mini",
	0x0090: "Stream Deck Mini MK.2",
	0x006c: "Stream Deck XL",
	0x008f: "Stream Deck XL V2",
	0x0084: "Stream Deck +",
	0x0086: "Stream Deck Pedal",
	0x009a: "Stream Deck Neo",
}

func ModelByPID(pid uint16) (Model, error) {
	for _, m := range Models {
		if m.ProductID == pid {
			return m, nil
		}
	}
	if name, ok := unsupported[pid]; ok {
		return Model{}, fmt.Errorf("%s (0x%04x) is not supported — this OS targets the Stream Deck MK.2", name, pid)
	}
	return Model{}, fmt.Errorf("unknown Elgato product 0x%04x", pid)
}

const (
	imageReportLen   = 1024
	imageHeaderLen   = 8
	imagePayloadLen  = imageReportLen - imageHeaderLen
	featureReportLen = 32
	keyStateOffset   = 4
)

// ImageReports splits a JPEG into the output reports for one key.
func ImageReports(key int, img []byte) [][]byte {
	var reports [][]byte
	for page := 0; ; page++ {
		start := page * imagePayloadLen
		end := start + imagePayloadLen
		last := end >= len(img)
		if last {
			end = len(img)
		}
		chunk := img[start:end]
		r := make([]byte, imageReportLen)
		r[0] = 0x02
		r[1] = 0x07
		r[2] = byte(key)
		if last {
			r[3] = 1
		}
		r[4] = byte(len(chunk))
		r[5] = byte(len(chunk) >> 8)
		r[6] = byte(page)
		r[7] = byte(page >> 8)
		copy(r[imageHeaderLen:], chunk)
		reports = append(reports, r)
		if last {
			return reports
		}
	}
}

func brightnessReport(pct int) []byte {
	pct = max(0, min(100, pct))
	r := make([]byte, featureReportLen)
	r[0], r[1], r[2] = 0x03, 0x08, byte(pct)
	return r
}

func resetReport() []byte {
	r := make([]byte, featureReportLen)
	r[0], r[1] = 0x03, 0x02
	return r
}

// ParseKeys extracts key states from an input report. ok is false for
// reports that are not key events.
func ParseKeys(m Model, report []byte) (states []bool, ok bool) {
	if len(report) < keyStateOffset+m.Keys || report[0] != 0x01 {
		return nil, false
	}
	states = make([]bool, m.Keys)
	for i := range states {
		states[i] = report[keyStateOffset+i] != 0
	}
	return states, true
}

// parseHIDID parses the HID_ID line of a hidraw uevent file, e.g.
// "HID_ID=0003:00000FD9:00000080".
func parseHIDID(uevent string) (vendor, product uint16, err error) {
	for _, line := range strings.Split(uevent, "\n") {
		v, ok := strings.CutPrefix(strings.TrimSpace(line), "HID_ID=")
		if !ok {
			continue
		}
		parts := strings.Split(v, ":")
		if len(parts) != 3 {
			break
		}
		ven, err1 := strconv.ParseUint(parts[1], 16, 32)
		pro, err2 := strconv.ParseUint(parts[2], 16, 32)
		if err1 != nil || err2 != nil {
			break
		}
		return uint16(ven), uint16(pro), nil
	}
	return 0, 0, errors.New("no HID_ID in uevent")
}

// ErrNotFound means no supported Stream Deck is plugged in.
var ErrNotFound = errors.New("no Stream Deck found")
