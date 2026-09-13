//go:build linux

// Package webview wraps GTK4 and WebKitGTK 6.0 into a minimal, native window
// hosting a single web view.
package webview

/*
#cgo pkg-config: gtk4 webkitgtk-6.0
#include <stdlib.h>
#include "bridge.h"
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"unsafe"
)

// Handlers receives events emitted by the web view. Every callback runs on the
// GTK main thread.
type Handlers struct {
	// Message is called when the page posts to the "zealish" script handler.
	Message func(payload string)
	// Notification is called when the page requests a desktop notification.
	// Returning true suppresses WebKit's built-in notification.
	Notification func(id uint64, title, body string) bool
	// NotificationClosed is called when a previously shown notification is
	// dismissed by the page.
	NotificationClosed func(id uint64)
	// ExternalURI is called for navigations that must open in the system
	// browser instead of the web view.
	ExternalURI func(uri string)
	// CloseRequest is called when the user closes the window. Returning true
	// keeps the window alive (hidden to tray).
	CloseRequest func() bool
	// GeometryChanged is called when the window is resized or (un)maximized.
	GeometryChanged func(width, height int, maximized bool)
}

// Options configures the web view at construction time.
type Options struct {
	Title      string
	URL        string
	DataDir    string
	CacheDir   string
	UserScript string
	Width      int
	Height     int
	Maximized  bool
	Hidden     bool
	Handlers   Handlers
}

// View is a native window with a single embedded WebKit web view.
type View struct {
	ptr    *C.ZwApp
	handle cgo.Handle
	opts   Options
}

// New creates the GTK application, window and web view. It must be called from
// the main OS thread.
func New(opts Options) (*View, error) {
	v := &View{opts: opts}
	v.handle = cgo.NewHandle(v)

	cfg := C.ZwConfig{
		title:       C.CString(opts.Title),
		url:         C.CString(opts.URL),
		data_dir:    C.CString(opts.DataDir),
		cache_dir:   C.CString(opts.CacheDir),
		user_script: C.CString(opts.UserScript),
		width:       C.int(opts.Width),
		height:      C.int(opts.Height),
		maximized:   cbool(opts.Maximized),
		hidden:      cbool(opts.Hidden),
		user_data:   C.uintptr_t(v.handle),
	}
	defer func() {
		C.free(unsafe.Pointer(cfg.title))
		C.free(unsafe.Pointer(cfg.url))
		C.free(unsafe.Pointer(cfg.data_dir))
		C.free(unsafe.Pointer(cfg.cache_dir))
		C.free(unsafe.Pointer(cfg.user_script))
	}()

	v.ptr = C.zw_app_new(&cfg)
	if v.ptr == nil {
		v.handle.Delete()
		return nil, errors.New("webview: failed to create application")
	}
	return v, nil
}

// Run starts the GTK main loop and blocks until the application quits.
func (v *View) Run() int {
	defer v.handle.Delete()
	return int(C.zw_app_run(v.ptr))
}

// Quit terminates the GTK main loop.
func (v *View) Quit() { C.zw_app_quit(v.ptr) }

// Show presents the window and gives it focus.
func (v *View) Show() { C.zw_app_show(v.ptr) }

// Hide withdraws the window without destroying the web view.
func (v *View) Hide() { C.zw_app_hide(v.ptr) }

// Visible reports whether the window is currently mapped.
func (v *View) Visible() bool { return C.zw_app_visible(v.ptr) != 0 }

// Reload reloads the current page.
func (v *View) Reload() { C.zw_app_reload(v.ptr) }

// NotificationClicked forwards a click on a native notification back to the
// originating page.
func (v *View) NotificationClicked(id uint64) {
	C.zw_app_notification_clicked(v.ptr, C.uint64_t(id))
}

// Eval runs a script from the trusted bridge world. Only scripts built by this
// application are ever passed here.
func (v *View) Eval(script string) {
	cs := C.CString(script)
	defer C.free(unsafe.Pointer(cs))
	C.zw_app_eval(v.ptr, cs)
}

// Geometry returns the current window size and maximized state.
func (v *View) Geometry() (width, height int, maximized bool) {
	var w, h, m C.int
	C.zw_app_geometry(v.ptr, &w, &h, &m)
	return int(w), int(h), m != 0
}

// Dispatch schedules fn to run on the GTK main thread.
func Dispatch(fn func()) {
	h := cgo.NewHandle(fn)
	C.zw_dispatch(C.uintptr_t(h))
}

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

func viewFrom(data C.uintptr_t) *View {
	return cgo.Handle(data).Value().(*View)
}

//export zwGoMessage
func zwGoMessage(data C.uintptr_t, payload *C.char) {
	v := viewFrom(data)
	if v.opts.Handlers.Message != nil {
		v.opts.Handlers.Message(C.GoString(payload))
	}
}

//export zwGoNotification
func zwGoNotification(data C.uintptr_t, id C.guint64, title, body *C.char) C.int {
	v := viewFrom(data)
	if v.opts.Handlers.Notification == nil {
		return 0
	}
	return cbool(v.opts.Handlers.Notification(uint64(id), C.GoString(title), C.GoString(body)))
}

//export zwGoNotificationClosed
func zwGoNotificationClosed(data C.uintptr_t, id C.guint64) {
	v := viewFrom(data)
	if v.opts.Handlers.NotificationClosed != nil {
		v.opts.Handlers.NotificationClosed(uint64(id))
	}
}

//export zwGoExternalURI
func zwGoExternalURI(data C.uintptr_t, uri *C.char) {
	v := viewFrom(data)
	if v.opts.Handlers.ExternalURI != nil {
		v.opts.Handlers.ExternalURI(C.GoString(uri))
	}
}

//export zwGoCloseRequest
func zwGoCloseRequest(data C.uintptr_t) C.int {
	v := viewFrom(data)
	if v.opts.Handlers.CloseRequest == nil {
		return 0
	}
	return cbool(v.opts.Handlers.CloseRequest())
}

//export zwGoGeometryChanged
func zwGoGeometryChanged(data C.uintptr_t, width, height, maximized C.int) {
	v := viewFrom(data)
	if v.opts.Handlers.GeometryChanged != nil {
		v.opts.Handlers.GeometryChanged(int(width), int(height), maximized != 0)
	}
}

//export zwGoDispatch
func zwGoDispatch(data C.uintptr_t) {
	h := cgo.Handle(data)
	fn, ok := h.Value().(func())
	h.Delete()
	if ok {
		fn()
	}
}
