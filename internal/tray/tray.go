//go:build linux

// Package tray implements a StatusNotifierItem tray icon with a DBusMenu,
// which is the protocol used by modern Linux desktops and works alongside
// GTK4 (unlike the GTK3-only AppIndicator library).
package tray

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	watcherName = "org.kde.StatusNotifierWatcher"
	watcherPath = "/StatusNotifierWatcher"
	itemIface   = "org.kde.StatusNotifierItem"
	itemPath    = "/StatusNotifierItem"
	menuIface   = "com.canonical.dbusmenu"
	menuPath    = "/MenuBar"
)

// Item is a tray menu entry.
type Item struct {
	Label   string
	Checked *bool // non-nil renders a checkmark toggle
	OnClick func()
}

// Tray owns the tray icon and its menu.
type Tray struct {
	conn      *dbus.Conn
	iconName  string
	title     string
	onPrimary func()

	mu    sync.Mutex
	items []Item

	menu     *menuServer
	revision uint32
}

// New registers a tray icon. onPrimary is invoked when the icon is activated.
func New(iconName, title string, items []Item, onPrimary func()) (*Tray, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}

	t := &Tray{
		conn:      conn,
		iconName:  iconName,
		title:     title,
		onPrimary: onPrimary,
		items:     items,
		revision:  1,
	}
	t.menu = &menuServer{tray: t}

	name := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
	reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		_ = conn.Close()
		return nil, errors.New("tray: bus name already taken")
	}

	if err := t.export(name); err != nil {
		_ = conn.Close()
		return nil, err
	}

	call := conn.Object(watcherName, dbus.ObjectPath(watcherPath)).
		Call(watcherName+".RegisterStatusNotifierItem", 0, name)
	if call.Err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("tray: no StatusNotifierWatcher: %w", call.Err)
	}

	return t, nil
}

// SetChecked updates the checkbox state of the item with the given label and
// refreshes the menu.
func (t *Tray) SetChecked(label string, checked bool) {
	t.mu.Lock()
	for i := range t.items {
		if t.items[i].Label == label && t.items[i].Checked != nil {
			v := checked
			t.items[i].Checked = &v
		}
	}
	t.revision++
	rev := t.revision
	t.mu.Unlock()

	_ = t.conn.Emit(dbus.ObjectPath(menuPath), menuIface+".LayoutUpdated", rev, int32(0))
}

// Close unregisters the tray icon.
func (t *Tray) Close() error { return t.conn.Close() }

func (t *Tray) export(busName string) error {
	item := &itemServer{tray: t}
	if err := t.conn.Export(item, dbus.ObjectPath(itemPath), itemIface); err != nil {
		return err
	}
	if err := t.conn.Export(t.menu, dbus.ObjectPath(menuPath), menuIface); err != nil {
		return err
	}

	itemProps := map[string]map[string]*prop.Prop{
		itemIface: {
			"Category":   ro("ApplicationStatus"),
			"Id":         ro("zealish-wa"),
			"Title":      ro(t.title),
			"Status":     ro("Active"),
			"IconName":   ro(t.iconName),
			"ToolTip":    ro(tooltip{IconName: t.iconName, Title: t.title}),
			"ItemIsMenu": ro(true),
			"Menu":       ro(dbus.ObjectPath(menuPath)),
		},
	}
	if _, err := prop.Export(t.conn, dbus.ObjectPath(itemPath), itemProps); err != nil {
		return err
	}

	menuProps := map[string]map[string]*prop.Prop{
		menuIface: {
			"Version":       ro(uint32(3)),
			"Status":        ro("normal"),
			"TextDirection": ro("ltr"),
			"IconThemePath": ro([]string{}),
		},
	}
	if _, err := prop.Export(t.conn, dbus.ObjectPath(menuPath), menuProps); err != nil {
		return err
	}

	node := introspect.Node{
		Name:       itemPath,
		Interfaces: []introspect.Interface{introspect.IntrospectData, prop.IntrospectData},
	}
	_ = busName
	return t.conn.Export(introspect.NewIntrospectable(&node), dbus.ObjectPath(itemPath),
		"org.freedesktop.DBus.Introspectable")
}

type tooltip struct {
	IconName string
	IconData [][]interface{}
	Title    string
	Body     string
}

func ro(v interface{}) *prop.Prop {
	return &prop.Prop{Value: v, Writable: false, Emit: prop.EmitTrue}
}

type itemServer struct{ tray *Tray }

func (s *itemServer) Activate(x, y int32) *dbus.Error {
	if s.tray.onPrimary != nil {
		s.tray.onPrimary()
	}
	return nil
}

func (s *itemServer) SecondaryActivate(x, y int32) *dbus.Error { return nil }

func (s *itemServer) Scroll(delta int32, orientation string) *dbus.Error { return nil }

type menuServer struct{ tray *Tray }

// GetLayout builds the menu tree. Item ids are 1-based indices into t.items.
func (m *menuServer) GetLayout(parentID, depth int32, propertyNames []string) (uint32, menuNode, *dbus.Error) {
	m.tray.mu.Lock()
	items := make([]Item, len(m.tray.items))
	copy(items, m.tray.items)
	rev := m.tray.revision
	m.tray.mu.Unlock()

	children := make([]dbus.Variant, 0, len(items))
	for i, it := range items {
		props := map[string]dbus.Variant{
			"label": dbus.MakeVariant(it.Label),
		}
		if it.Label == "" {
			props["type"] = dbus.MakeVariant("separator")
		} else if it.Checked != nil {
			props["toggle-type"] = dbus.MakeVariant("checkmark")
			state := 0
			if *it.Checked {
				state = 1
			}
			props["toggle-state"] = dbus.MakeVariant(int32(state))
		}
		children = append(children, dbus.MakeVariant(menuNode{
			ID:       int32(i + 1),
			Props:    props,
			Children: []dbus.Variant{},
		}))
	}

	return rev, menuNode{
		ID:       0,
		Props:    map[string]dbus.Variant{"children-display": dbus.MakeVariant("submenu")},
		Children: children,
	}, nil
}

func (m *menuServer) GetGroupProperties(ids []int32, propertyNames []string) ([]struct {
	ID    int32
	Props map[string]dbus.Variant
}, *dbus.Error) {
	return nil, nil
}

func (m *menuServer) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	return dbus.MakeVariant(""), nil
}

func (m *menuServer) Event(id int32, eventID string, data dbus.Variant, timestamp uint32) *dbus.Error {
	if eventID != "clicked" {
		return nil
	}

	m.tray.mu.Lock()
	var fn func()
	if idx := int(id) - 1; idx >= 0 && idx < len(m.tray.items) {
		fn = m.tray.items[idx].OnClick
	}
	m.tray.mu.Unlock()

	if fn != nil {
		fn()
	}
	return nil
}

func (m *menuServer) EventGroup(events []struct {
	ID        int32
	EventID   string
	Data      dbus.Variant
	Timestamp uint32
}) ([]int32, *dbus.Error) {
	for _, e := range events {
		_ = m.Event(e.ID, e.EventID, e.Data, e.Timestamp)
	}
	return nil, nil
}

func (m *menuServer) AboutToShow(id int32) (bool, *dbus.Error) { return false, nil }

type menuNode struct {
	ID       int32
	Props    map[string]dbus.Variant
	Children []dbus.Variant
}
