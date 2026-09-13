// Package bridge defines the minimal JavaScript <-> Go protocol. The bridge
// exposes exactly three inbound calls and never evaluates page-supplied code.
package bridge

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
)

//go:embed bridge.js
var userScript string

// UserScript returns the JavaScript injected at document start.
func UserScript() string { return userScript }

// Kind enumerates the messages the page may send to Go.
type Kind string

const (
	// KindReady signals the WhatsApp UI finished booting.
	KindReady Kind = "ready"
	// KindUnread reports the current unread conversation count.
	KindUnread Kind = "unread"
	// KindOpenExternal requests a URL be opened in the system browser.
	KindOpenExternal Kind = "open_external"
)

// Message is a decoded payload sent from the page.
type Message struct {
	Type   Kind   `json:"type"`
	URL    string `json:"url,omitempty"`
	Unread int    `json:"unread,omitempty"`
}

// Decode parses a raw script-message payload.
func Decode(payload string) (Message, error) {
	var msg Message
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		return Message{}, err
	}
	switch msg.Type {
	case KindReady, KindUnread, KindOpenExternal:
		return msg, nil
	default:
		return Message{}, fmt.Errorf("bridge: unknown message type %q", msg.Type)
	}
}

// FocusChat builds the script that focuses a chat by notification tag. The tag
// is JSON-quoted, so it can never escape the string literal.
func FocusChat(tag string) string {
	return "window.__zealish && window.__zealish.focusChat(" + strconv.Quote(tag) + ");"
}
