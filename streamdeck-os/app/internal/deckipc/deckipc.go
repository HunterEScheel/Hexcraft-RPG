// Package deckipc links the on-screen button editor (console) with the
// daemon that owns the Stream Deck: the editor shows unsaved changes on the
// deck as a live preview, and deck key presses select keys in the editor
// instead of running their actions.
package deckipc

import (
	"bufio"
	"encoding/json"
	"net"
	"sync"
	"time"

	"streamdeckd/internal/config"
)

// Socket is where the daemon listens.
var Socket = "/run/streamdeckd.sock"

// Msg is one JSON line in either direction.
//
//	editor -> daemon: {"op":"preview","page":"home","layout":{...},"select":3}
//	daemon -> editor: {"op":"press","key":3}
type Msg struct {
	Op     string         `json:"op"`
	Page   string         `json:"page,omitempty"`
	Layout *config.Layout `json:"layout,omitempty"`
	Select int            `json:"select"`
	Key    int            `json:"key"`
}

// Client is the editor's end.
type Client struct {
	conn    net.Conn
	mu      sync.Mutex
	enc     *json.Encoder
	Presses chan int
}

// Dial connects to the daemon. The deck switches to edit mode until Close.
func Dial() (*Client, error) {
	conn, err := net.DialTimeout("unix", Socket, 2*time.Second)
	if err != nil {
		return nil, err
	}
	c := &Client{conn: conn, enc: json.NewEncoder(conn), Presses: make(chan int, 16)}
	go func() {
		sc := bufio.NewScanner(conn)
		for sc.Scan() {
			var m Msg
			if json.Unmarshal(sc.Bytes(), &m) == nil && m.Op == "press" {
				select {
				case c.Presses <- m.Key:
				default:
				}
			}
		}
		close(c.Presses)
	}()
	return c, nil
}

// Preview shows layout's page on the deck with key sel outlined (-1: none).
func (c *Client) Preview(l config.Layout, page string, sel int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	return c.enc.Encode(Msg{Op: "preview", Page: page, Layout: &l, Select: sel})
}

func (c *Client) Close() error { return c.conn.Close() }
