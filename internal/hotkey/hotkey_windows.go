//go:build windows

package hotkey

import (
	"fmt"
	"strings"
	"sync"
		"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	procRegHotKey    = user32.NewProc("RegisterHotKey")
	procUnregHotKey  = user32.NewProc("UnregisterHotKey")
	procGetMessage   = user32.NewProc("GetMessageW")
)

const (
	MOD_ALT      = 0x0001
	MOD_CONTROL  = 0x0002
	MOD_SHIFT    = 0x0004
	MOD_WIN      = 0x0008
	MOD_NOREPEAT = 0x4000
	WM_HOTKEY    = 0x0312
)

type MSG struct {
	HWND    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type Manager struct {
	mu       sync.Mutex
	handlers map[int32]func()
	nextID   int32
}

func NewManager() *Manager {
	return &Manager{
		handlers: make(map[int32]func()),
		nextID:   1,
	}
}

// parseHotkey parses "ctrl+shift+r" → (modifiers, vkCode, error)
func parseHotkey(combo string) (uint32, uint32, error) {
	combo = strings.NewReplacer("<ctrl>", "ctrl", "<control>", "ctrl", "<shift>", "shift", "<alt>", "alt", "<cmd>", "cmd").Replace(strings.ToLower(combo))
	parts := strings.Split(combo, "+")
	var mods uint32
	var vk uint32
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch p {
		case "ctrl", "control":
			mods |= MOD_CONTROL
		case "shift":
			mods |= MOD_SHIFT
		case "alt":
			mods |= MOD_ALT
		case "win", "super":
			mods |= MOD_WIN
		default:
			if len(p) == 1 {
				vk = uint32(strings.ToUpper(p)[0])
			} else {
				return 0, 0, fmt.Errorf("unknown key: %s", p)
			}
		}
	}
	mods |= MOD_NOREPEAT
	return mods, vk, nil
}

func (m *Manager) Register(combo string, handler func()) (int32, error) {
	mods, vk, err := parseHotkey(combo)
	if err != nil {
		return 0, err
	}
	m.mu.Lock()
	id := m.nextID
	m.nextID++
	m.handlers[id] = handler
	m.mu.Unlock()

	r, _, e := procRegHotKey.Call(0, uintptr(id), uintptr(mods), uintptr(vk))
	if r == 0 {
		return 0, fmt.Errorf("RegisterHotKey failed: %v", e)
	}
	return id, nil
}

func (m *Manager) Unregister(id int32) {
	procUnregHotKey.Call(0, uintptr(id))
	m.mu.Lock()
	delete(m.handlers, id)
	m.mu.Unlock()
}

// Listen runs the Windows message pump — call in a dedicated goroutine
func (m *Manager) Listen() {
	var msg MSG
	for {
		r, _, _ := procGetMessage.Call(
			uintptr(unsafe.Pointer(&msg)),
			0, 0, 0,
		)
		if r == 0 || r == ^uintptr(0) { // WM_QUIT or error
			break
		}
		if msg.Message == WM_HOTKEY {
			id := int32(msg.WParam)
			m.mu.Lock()
			h, ok := m.handlers[id]
			m.mu.Unlock()
			if ok {
				go h()
			}
		}
	}
}

// clipboard helpers via Win32
var (
	procOpenClipboard  = user32.NewProc("OpenClipboard")
	procCloseClipboard = user32.NewProc("CloseClipboard")
	procGetClipboard   = user32.NewProc("GetClipboardData")
	procSetClipboard   = user32.NewProc("SetClipboardData")
	procEmptyClipboard = user32.NewProc("EmptyClipboard")
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalAlloc    = kernel32.NewProc("GlobalAlloc")
	procGlobalLock     = kernel32.NewProc("GlobalLock")
	procGlobalUnlock   = kernel32.NewProc("GlobalUnlock")
	procLstrcpyW       = kernel32.NewProc("lstrcpyW")
)

const (
	CF_UNICODETEXT = 13
	GMEM_MOVEABLE  = 0x0002
)

func GetClipboard() string {
	procOpenClipboard.Call(0)
	defer procCloseClipboard.Call()
	h, _, _ := procGetClipboard.Call(CF_UNICODETEXT)
	if h == 0 {
		return ""
	}
	ptr, _, _ := procGlobalLock.Call(h)
	defer procGlobalUnlock.Call(h)
	return windows.UTF16PtrToString((*uint16)(unsafe.Pointer(ptr)))
}

func SetClipboard(text string) {
	utf16, err := windows.UTF16FromString(text)
	if err != nil {
		return
	}
	size := uintptr(len(utf16) * 2)
	h, _, _ := procGlobalAlloc.Call(GMEM_MOVEABLE, size)
	if h == 0 {
		return
	}
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), int(size))
	src := unsafe.Slice((*byte)(unsafe.Pointer(&utf16[0])), int(size))
	copy(dst, src)
	procGlobalUnlock.Call(h)

	procOpenClipboard.Call(0)
	procEmptyClipboard.Call()
	procSetClipboard.Call(CF_UNICODETEXT, h)
	procCloseClipboard.Call()
}

// SendCtrlC / SendCtrlV — simulate keypress to copy/paste selection
var procSendInput = user32.NewProc("SendInput")

const (
	INPUT_KEYBOARD    = 1
	KEYEVENTF_KEYUP   = 0x0002
	VK_CONTROL        = 0x11
	VK_C              = 0x43
	VK_V              = 0x56
)

type KEYBDINPUT struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type INPUT struct {
	Type uint32
	_    [4]byte // padding for 64-bit
	Ki   KEYBDINPUT
	_    [8]byte // fill to 28 bytes (size of INPUT on 64-bit)
}

func sendKey(vk uint16, up bool) {
	flags := uint32(0)
	if up {
		flags = KEYEVENTF_KEYUP
	}
	inp := INPUT{
		Type: INPUT_KEYBOARD,
		Ki:   KEYBDINPUT{WVk: vk, DwFlags: flags},
	}
	procSendInput.Call(1, uintptr(unsafe.Pointer(&inp)), unsafe.Sizeof(inp))
}

func SendCtrlC() {
	sendKey(VK_CONTROL, false)
	sendKey(VK_C, false)
	sendKey(VK_C, true)
	sendKey(VK_CONTROL, true)
}

func SendCtrlV() {
	sendKey(VK_CONTROL, false)
	sendKey(VK_V, false)
	sendKey(VK_V, true)
	sendKey(VK_CONTROL, true)
}
