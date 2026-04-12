//go:build windows

package tray

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procCreateWindowEx    = user32.NewProc("CreateWindowExW")
	procRegisterClassEx   = user32.NewProc("RegisterClassExW")
	procDefWindowProc     = user32.NewProc("DefWindowProcW")
	procGetMessage        = user32.NewProc("GetMessageW")
	procDispatchMessage   = user32.NewProc("DispatchMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procPostQuitMessage   = user32.NewProc("PostQuitMessage")
	procDestroyWindow     = user32.NewProc("DestroyWindow")
	procSetForegroundWin  = user32.NewProc("SetForegroundWindow")
	procCreatePopupMenu   = user32.NewProc("CreatePopupMenu")
	procAppendMenuW       = user32.NewProc("AppendMenuW")
	procTrackPopupMenu    = user32.NewProc("TrackPopupMenu")
	procDestroyMenu       = user32.NewProc("DestroyMenu")
	procGetCursorPos      = user32.NewProc("GetCursorPos")
	procLoadIcon          = user32.NewProc("LoadIconW")
	procShellNotify       = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandle   = kernel32.NewProc("GetModuleHandleW")
)

const (
	WM_USER          = 0x0400
	WM_TRAYICON      = WM_USER + 1
	WM_DESTROY       = 0x0002
	WM_COMMAND       = 0x0111
	WM_LBUTTONDBLCLK = 0x0203
	WM_RBUTTONUP     = 0x0205
	NIM_ADD          = 0x00000000
	NIM_DELETE       = 0x00000002
	NIF_MESSAGE      = 0x00000001
	NIF_ICON         = 0x00000002
	NIF_TIP          = 0x00000004
	IDI_APPLICATION  = 32512
	MF_STRING        = 0x00000000
	MF_SEPARATOR     = 0x00000800
	MF_GRAYED        = 0x00000001
	TPM_RIGHTBUTTON  = 0x0002
	TPM_RETURNCMD    = 0x0100
	CS_HREDRAW       = 0x0002
	CS_VREDRAW       = 0x0001
	WS_OVERLAPPED    = 0x00000000
)

type NOTIFYICONDATA struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type POINT struct{ X, Y int32 }

type MSG struct {
	HWND    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type MenuItem struct {
	Label     string
	Separator bool
	ID        uint32
	Action    func()
}

type Icon struct {
	hwnd    uintptr
	nid     NOTIFYICONDATA
	items   []MenuItem
	actions map[uint32]func()
	onOpen  func()
}

var globalIcon *Icon

func New(tooltip string, items []MenuItem, onOpen func()) *Icon {
	icon := &Icon{
		items:   items,
		actions: make(map[uint32]func()),
		onOpen:  onOpen,
	}
	for i := range items {
		if items[i].ID == 0 && !items[i].Separator {
			items[i].ID = uint32(100 + i)
		}
		if items[i].Action != nil {
			icon.actions[items[i].ID] = items[i].Action
		}
	}
	globalIcon = icon
	return icon
}

func wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_TRAYICON:
		switch lParam {
		case WM_LBUTTONDBLCLK:
			if globalIcon != nil && globalIcon.onOpen != nil {
				go globalIcon.onOpen()
			}
		case WM_RBUTTONUP:
			if globalIcon != nil {
				globalIcon.showMenu(hwnd)
			}
		}
		return 0
	case WM_COMMAND:
		id := uint32(wParam & 0xFFFF)
		if globalIcon != nil {
			if action, ok := globalIcon.actions[id]; ok {
				go action()
			}
		}
		return 0
	case WM_DESTROY:
		if globalIcon != nil {
			procShellNotify.Call(NIM_DELETE, uintptr(unsafe.Pointer(&globalIcon.nid)))
		}
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
}

func (ic *Icon) showMenu(hwnd uintptr) {
	hMenu, _, _ := procCreatePopupMenu.Call()
	for _, item := range ic.items {
		if item.Separator {
			procAppendMenuW.Call(hMenu, MF_SEPARATOR, 0, 0)
		} else {
			label, _ := windows.UTF16PtrFromString(item.Label)
			procAppendMenuW.Call(hMenu, MF_STRING, uintptr(item.ID), uintptr(unsafe.Pointer(label)))
		}
	}
	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWin.Call(hwnd)
	procTrackPopupMenu.Call(hMenu, TPM_RIGHTBUTTON, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
	procDestroyMenu.Call(hMenu)
}

func (ic *Icon) Run() error {
	hInst, _, _ := procGetModuleHandle.Call(0)
	className, _ := windows.UTF16PtrFromString("ITrayClass")

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		Style:         CS_HREDRAW | CS_VREDRAW,
		LpfnWndProc:   windows.NewCallback(wndProc),
		HInstance:     hInst,
		LpszClassName: className,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	winName, _ := windows.UTF16PtrFromString("ITrayWindow")
	hwnd, _, _ := procCreateWindowEx.Call(
		0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(winName)),
		WS_OVERLAPPED, 0, 0, 0, 0, 0, 0, hInst, 0,
	)
	ic.hwnd = hwnd

	// Load default app icon
	hIcon, _, _ := procLoadIcon.Call(0, IDI_APPLICATION)

	tipUTF16, _ := windows.UTF16FromString("Integrated Translator")
	ic.nid = NOTIFYICONDATA{
		CbSize:           uint32(unsafe.Sizeof(NOTIFYICONDATA{})),
		HWnd:             hwnd,
		UID:              1,
		UFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
		UCallbackMessage: WM_TRAYICON,
		HIcon:            hIcon,
	}
	copy(ic.nid.SzTip[:], tipUTF16)
	procShellNotify.Call(NIM_ADD, uintptr(unsafe.Pointer(&ic.nid)))

	var msg MSG
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || r == ^uintptr(0) {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
	return nil
}

func (ic *Icon) Stop() {
	procDestroyWindow.Call(ic.hwnd)
}
