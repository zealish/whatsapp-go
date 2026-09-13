//go:build linux

// Package notify sends desktop notifications over the freedesktop DBus
// notification interface.
package notify

import (
	"context"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	busName    = "org.freedesktop.Notifications"
	busPath    = "/org/freedesktop/Notifications"
	busIface   = "org.freedesktop.Notifications"
	appName    = "Zealish WhatsApp"
	defaultKey = "default"
)

// Notification describes a single desktop notification.
type Notification struct {
	Title string
	Body  string
	Icon  string
	// Tag identifies the source so repeated notifications replace each other.
	Tag string
}

// Notifier publishes notifications and reports user interaction.
type Notifier struct {
	conn *dbus.Conn
	obj  dbus.BusObject

	mu       sync.Mutex
	replaces map[string]uint32 // tag -> server notification id
	sources  map[uint32]string // server id -> tag

	onActivate func(tag string)
	onClose    func(tag string)
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// New connects to the session bus and starts listening for notification
// signals. OnActivate fires when a notification is clicked.
func New(onActivate, onClose func(tag string)) (*Notifier, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}

	n := &Notifier{
		conn:       conn,
		obj:        conn.Object(busName, dbus.ObjectPath(busPath)),
		replaces:   make(map[string]uint32),
		sources:    make(map[uint32]string),
		onActivate: onActivate,
		onClose:    onClose,
	}

	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface(busIface),
		dbus.WithMatchObjectPath(dbus.ObjectPath(busPath)),
	); err != nil {
		_ = conn.Close()
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	n.cancel = cancel

	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)

	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		n.listen(ctx, signals)
	}()

	return n, nil
}

// Send shows a notification, replacing any earlier notification with the same
// tag.
func (n *Notifier) Send(msg Notification) error {
	n.mu.Lock()
	replaces := n.replaces[msg.Tag]
	n.mu.Unlock()

	hints := map[string]dbus.Variant{
		"desktop-entry": dbus.MakeVariant("com.zealish.WhatsApp"),
		"urgency":       dbus.MakeVariant(byte(1)),
	}
	actions := []string{defaultKey, "Open"}

	var id uint32
	call := n.obj.Call(busIface+".Notify", 0,
		appName, replaces, msg.Icon, msg.Title, msg.Body, actions, hints, int32(-1))
	if call.Err != nil {
		return call.Err
	}
	if err := call.Store(&id); err != nil {
		return err
	}

	n.mu.Lock()
	if msg.Tag != "" {
		n.replaces[msg.Tag] = id
	}
	n.sources[id] = msg.Tag
	n.mu.Unlock()

	return nil
}

// Close stops the signal listener and releases the bus connection.
func (n *Notifier) Close() error {
	n.cancel()
	err := n.conn.Close()
	n.wg.Wait()
	return err
}

func (n *Notifier) listen(ctx context.Context, signals chan *dbus.Signal) {
	for {
		select {
		case <-ctx.Done():
			return
		case sig, ok := <-signals:
			if !ok {
				return
			}
			switch sig.Name {
			case busIface + ".ActionInvoked":
				n.handle(sig, n.onActivate)
			case busIface + ".NotificationClosed":
				n.handle(sig, n.onClose)
			}
		}
	}
}

func (n *Notifier) handle(sig *dbus.Signal, fn func(string)) {
	if len(sig.Body) == 0 {
		return
	}
	id, ok := sig.Body[0].(uint32)
	if !ok {
		return
	}

	n.mu.Lock()
	tag, known := n.sources[id]
	if sig.Name == busIface+".NotificationClosed" {
		delete(n.sources, id)
		if tag != "" && n.replaces[tag] == id {
			delete(n.replaces, tag)
		}
	}
	n.mu.Unlock()

	if known && fn != nil {
		fn(tag)
	}
}
