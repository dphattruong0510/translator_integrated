//go:build windows

package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ────────────────────────────────────────────────────────────────────────────
// Win32 bindings
// ────────────────────────────────────────────────────────────────────────────

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")

	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
	procGetMessage       = user32.NewProc("GetMessageW")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procPostMessage      = user32.NewProc("PostMessageW")
	procSendMessage      = user32.NewProc("SendMessageW")
	procSetWindowText    = user32.NewProc("SetWindowTextW")
	procGetWindowText    = user32.NewProc("GetWindowTextW")
	procCreateFont       = gdi32.NewProc("CreateFontW")
	procSelectObject     = gdi32.NewProc("SelectObject")
	procDeleteObject     = gdi32.NewProc("DeleteObject")
	procSetBkMode        = gdi32.NewProc("SetBkMode")
	procSetTextColor     = gdi32.NewProc("SetTextColor")
	procBeginPaint       = user32.NewProc("BeginPaint")
	procEndPaint         = user32.NewProc("EndPaint")
	procFillRect         = user32.NewProc("FillRect")
	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	procDrawText         = user32.NewProc("DrawTextW")
	procGetClientRect    = user32.NewProc("GetClientRect")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procSetTimer         = user32.NewProc("SetTimerW")
	procKillTimer        = user32.NewProc("KillTimerW")
	procLoadCursor       = user32.NewProc("LoadCursorW")
	procSetLayeredWin    = user32.NewProc("SetLayeredWindowAttributes")
	procGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
	procMessageBox       = user32.NewProc("MessageBoxW")
	procCreateEdit       = user32.NewProc("CreateWindowExW") // alias for clarity
)

const (
	WS_OVERLAPPED       = 0x00000000
	WS_POPUP            = 0x80000000
	WS_CHILD            = 0x40000000
	WS_VISIBLE          = 0x10000000
	WS_BORDER           = 0x00800000
	WS_VSCROLL          = 0x00200000
	WS_TABSTOP          = 0x00010000
	WS_EX_TOPMOST       = 0x00000008
	WS_EX_TOOLWINDOW    = 0x00000080
	WS_EX_LAYERED       = 0x00080000
	WS_EX_APPWINDOW     = 0x00040000
	WS_EX_DLGMODALFRAME = 0x00000001
	ES_MULTILINE        = 0x0004
	ES_AUTOVSCROLL      = 0x0040
	ES_READONLY         = 0x0800
	ES_PASSWORD         = 0x0020
	BS_PUSHBUTTON       = 0x00000000
	BS_FLAT             = 0x00008000
	SS_LEFT             = 0x00000000
	SS_CENTER           = 0x00000001
	SW_SHOW             = 5
	SW_HIDE             = 0
	WM_CLOSE            = 0x0010
	WM_DESTROY          = 0x0002
	WM_COMMAND          = 0x0111
	WM_TIMER            = 0x0113
	WM_PAINT            = 0x000F
	WM_CTLCOLORSTATIC   = 0x0138
	WM_CTLCOLOREDIT     = 0x0133
	WM_CTLCOLORBTN      = 0x0135
	WM_ERASEBKGND       = 0x0014
	BN_CLICKED          = 0
	LWA_ALPHA           = 0x2
	DT_LEFT             = 0x00000000
	DT_WORDBREAK        = 0x00000010
	DT_NOCLIP           = 0x00000100
	MB_OK               = 0x00000000
	MB_ICONINFORMATION  = 0x00000040
	MB_ICONWARNING      = 0x00000030
	MB_ICONERROR        = 0x00000010
	SM_CXSCREEN         = 0
	SM_CYSCREEN         = 1
	TRANSPARENT_BK      = 1
	TRANSPARENT         = 1
	OPAQUE              = 2
	SWP_NOZORDER        = 0x0004
	SWP_NOACTIVATE      = 0x0010
	HWND_TOPMOST        = ^uintptr(0) // -1
)

type RECT struct{ Left, Top, Right, Bottom int32 }
type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}
type POINT struct{ X, Y int32 }

// color helpers: Win32 uses BGR
func rgb(r, g, b byte) uintptr {
	return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16
}

var (
	colorBG     = rgb(11, 16, 32)    // #0b1020
	colorCard   = rgb(17, 24, 39)    // #111827
	colorAccent = rgb(79, 70, 229)   // #4f46e5
	colorText   = rgb(255, 255, 255) // #ffffff
	colorMuted  = rgb(148, 163, 184) // #94a3b8
	colorBorder = rgb(36, 48, 69)    // #243045

	brushBG     uintptr
	brushCard   uintptr
	brushAccent uintptr
	hFont       uintptr
	hFontBold   uintptr
	hFontSmall  uintptr
	initOnce    sync.Once
)

func initResources() {
	initOnce.Do(func() {
		brushBG, _, _ = procCreateSolidBrush.Call(colorBG)
		brushCard, _, _ = procCreateSolidBrush.Call(colorCard)
		brushAccent, _, _ = procCreateSolidBrush.Call(colorAccent)
		fontName, _ := windows.UTF16PtrFromString("Segoe UI")
		hFont, _, _ = procCreateFont.Call(
			16, 0, 0, 0, 400, 0, 0, 0, 0, 0, 0, 0, 0,
			uintptr(unsafe.Pointer(fontName)),
		)
		hFontBold, _, _ = procCreateFont.Call(
			16, 0, 0, 0, 700, 0, 0, 0, 0, 0, 0, 0, 0,
			uintptr(unsafe.Pointer(fontName)),
		)
		hFontSmall, _, _ = procCreateFont.Call(
			13, 0, 0, 0, 400, 0, 0, 0, 0, 0, 0, 0, 0,
			uintptr(unsafe.Pointer(fontName)),
		)
	})
}

func str(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
}

func hInst() uintptr {
	h, _, _ := procGetModuleHandle.Call(0)
	return h
}

func screenW() int32 {
	r, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
	return int32(r)
}
func screenH() int32 {
	r, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)
	return int32(r)
}

// ────────────────────────────────────────────────────────────────────────────
// Popup (hover / info / loading modes)
// ────────────────────────────────────────────────────────────────────────────

type popupState struct {
	hwnd        uintptr
	editHwnd    uintptr
	mu          sync.Mutex
	autoCloseMs int
	timerID     uintptr
}

var currentPopup *popupState
var popupMu sync.Mutex

func ShowPopup(title, original, translated string, autoCloseMs int) {
	ClosePopup()
	go createPopup(title, original, translated, autoCloseMs)
}

func ClosePopup() {
	popupMu.Lock()
	p := currentPopup
	popupMu.Unlock()
	if p != nil && p.hwnd != 0 {
		procDestroyWindow.Call(p.hwnd)
	}
}

func createPopup(title, original, translated string, autoCloseMs int) {
	initResources()

	const (
		w = 440
		h = 200
	)

	var cx, cy uintptr
	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	cx = uintptr(pt.X) + 14
	cy = uintptr(pt.Y) + 14
	if int32(cx)+w > screenW() {
		cx = uintptr(screenW() - w - 12)
	}
	if int32(cy)+h > screenH() {
		cy = uintptr(screenH() - h - 40)
	}

	className := str("ITPopup")
	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   windows.NewCallback(popupWndProc),
		HInstance:     hInst(),
		HbrBackground: brushBG,
		LpszClassName: className,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := procCreateWindowEx.Call(
		WS_EX_TOPMOST|WS_EX_TOOLWINDOW|WS_EX_LAYERED,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(str(title))),
		WS_POPUP|WS_VISIBLE,
		cx, cy, w, h,
		0, 0, hInst(), 0,
	)
	// Alpha ~92%
	procSetLayeredWin.Call(hwnd, 0, 235, LWA_ALPHA)

	// Title bar label (static)
	hLabel, _, _ := procCreateWindowEx.Call(
		0, uintptr(unsafe.Pointer(str("STATIC"))), uintptr(unsafe.Pointer(str(title))),
		WS_CHILD|WS_VISIBLE|SS_LEFT,
		10, 6, 320, 20,
		hwnd, 0, hInst(), 0,
	)
	procSendMessage.Call(hLabel, 0x0030 /*WM_SETFONT*/, hFontBold, 1)

	// Close button
	closeID := uintptr(901)
	hClose, _, _ := procCreateWindowEx.Call(
		0, uintptr(unsafe.Pointer(str("BUTTON"))), uintptr(unsafe.Pointer(str("✕"))),
		WS_CHILD|WS_VISIBLE|BS_PUSHBUTTON|BS_FLAT,
		w-34, 4, 28, 24,
		hwnd, closeID, hInst(), 0,
	)
	procSendMessage.Call(hClose, 0x0030, hFont, 1)

	// Original text label
	origY := int32(38)
	if original != "" {
		truncated := truncate("Original: "+original, 80)
		hOrig, _, _ := procCreateWindowEx.Call(
			0, uintptr(unsafe.Pointer(str("STATIC"))), uintptr(unsafe.Pointer(str(truncated))),
			WS_CHILD|WS_VISIBLE|SS_LEFT,
			10, uintptr(origY), w-20, 18,
			hwnd, 0, hInst(), 0,
		)
		procSendMessage.Call(hOrig, 0x0030, hFontSmall, 1)
		origY += 22
	}

	// Translation label
	hTransLbl, _, _ := procCreateWindowEx.Call(
		0, uintptr(unsafe.Pointer(str("STATIC"))), uintptr(unsafe.Pointer(str("Translation:"))),
		WS_CHILD|WS_VISIBLE|SS_LEFT,
		10, uintptr(origY), 120, 18,
		hwnd, 0, hInst(), 0,
	)
	procSendMessage.Call(hTransLbl, 0x0030, hFontBold, 1)
	origY += 22

	// Translation edit (read-only, multiline)
	editH := h - int(origY) - 12
	hEdit, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(str("EDIT"))),
		uintptr(unsafe.Pointer(str(translated))),
		WS_CHILD|WS_VISIBLE|ES_MULTILINE|ES_AUTOVSCROLL|ES_READONLY|WS_VSCROLL,
		10, uintptr(origY), w-20, uintptr(editH),
		hwnd, 0, hInst(), 0,
	)
	procSendMessage.Call(hEdit, 0x0030, hFont, 1)

	state := &popupState{
		hwnd:        hwnd,
		editHwnd:    hEdit,
		autoCloseMs: autoCloseMs,
	}
	popupMu.Lock()
	currentPopup = state
	popupMu.Unlock()

	if autoCloseMs > 0 {
		state.timerID = 1
		procSetTimer.Call(hwnd, 1, uintptr(autoCloseMs), 0)
	}

	procShowWindow.Call(hwnd, SW_SHOW)
	procUpdateWindow.Call(hwnd)

	// message pump for this popup
	var msg MSG
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), hwnd, 0, 0)
		if r == 0 || r == ^uintptr(0) {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func popupWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		if wParam>>16 == BN_CLICKED {
			id := wParam & 0xFFFF
			if id == 901 { // close button
				procDestroyWindow.Call(hwnd)
			}
		}
	case WM_TIMER:
		procKillTimer.Call(hwnd, wParam)
		procDestroyWindow.Call(hwnd)
	case WM_CTLCOLORSTATIC:
		hdc := wParam
		procSetBkMode.Call(hdc, TRANSPARENT)
		procSetTextColor.Call(hdc, colorMuted)
		return brushBG
	case WM_CTLCOLOREDIT:
		hdc := wParam
		procSetBkMode.Call(hdc, OPAQUE)
		procSetTextColor.Call(hdc, colorText)
		return brushCard
	case WM_ERASEBKGND:
		// paint background + accent bar
		hdc := wParam
		var rc RECT
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), brushBG)
		barRC := RECT{0, 0, rc.Right, 32}
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&barRC)), brushAccent)
		return 1
	case WM_DESTROY:
		popupMu.Lock()
		currentPopup = nil
		popupMu.Unlock()
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
}

// ────────────────────────────────────────────────────────────────────────────
// Settings Window
// ────────────────────────────────────────────────────────────────────────────

type SettingsValues struct {
	MyLang      string
	PartnerLang string
	AutoDetect  bool
	AIEnhanced  bool
	AIProvider  string
	OpenAIKey   string
	ClaudeKey   string
	GeminiKey   string
	GeminiModel string
	DeepLKey    string
	LibreURL    string
	HotkeyIn    string
	HotkeyOut   string
}

type settingsState struct {
	hwnd      uintptr
	fields    map[string]uintptr // field name → edit hwnd
	onSave    func(SettingsValues)
	onTest    func(provider string) (bool, string)
	mu        sync.Mutex
}

var Languages = []struct{ Name, Code string }{
	{"Vietnamese", "vi"}, {"Korean", "ko"}, {"English", "en"},
	{"Japanese", "ja"}, {"Chinese", "zh"}, {"French", "fr"},
	{"German", "de"}, {"Spanish", "es"}, {"Thai", "th"},
}

var LangCodeToName = map[string]string{
	"vi": "Vietnamese", "ko": "Korean", "en": "English",
	"ja": "Japanese", "zh": "Chinese", "fr": "French",
	"de": "German", "es": "Spanish", "th": "Thai",
}

var globalSettings *settingsState

func ShowSettings(initial SettingsValues, onSave func(SettingsValues), onTest func(string) (bool, string)) {
	if globalSettings != nil {
		procShowWindow.Call(globalSettings.hwnd, SW_SHOW)
		return
	}
	go createSettings(initial, onSave, onTest)
}

func createSettings(initial SettingsValues, onSave func(SettingsValues), onTest func(string) (bool, string)) {
	initResources()

	const (
		w = 600
		h = 640
	)
	sw := screenW()
	sh := screenH()
	x := (sw - w) / 2
	y := (sh - h) / 2

	className := str("ITSettings")
	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   windows.NewCallback(settingsWndProc),
		HInstance:     hInst(),
		HbrBackground: brushBG,
		LpszClassName: className,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := procCreateWindowEx.Call(
		WS_EX_APPWINDOW|WS_EX_DLGMODALFRAME,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(str("Integrated Translator — Settings"))),
		WS_POPUP|WS_VISIBLE|WS_BORDER,
		uintptr(x), uintptr(y), w, h,
		0, 0, hInst(), 0,
	)

	s := &settingsState{
		hwnd:   hwnd,
		fields: make(map[string]uintptr),
		onSave: onSave,
		onTest: onTest,
	}
	globalSettings = s

	y0 := 48 // start below header
	addLabel := func(label string, yy int) {
		h0, _, _ := procCreateWindowEx.Call(
			0, uintptr(unsafe.Pointer(str("STATIC"))), uintptr(unsafe.Pointer(str(label))),
			WS_CHILD|WS_VISIBLE|SS_LEFT,
			20, uintptr(yy), 180, 18,
			hwnd, 0, hInst(), 0,
		)
		procSendMessage.Call(h0, 0x0030, hFont, 1)
	}
	addEdit := func(name, value string, yy int, password bool) uintptr {
		flags := uintptr(WS_CHILD | WS_VISIBLE | WS_BORDER)
		if password {
			flags |= ES_PASSWORD
		}
		he, _, _ := procCreateWindowEx.Call(
			0, uintptr(unsafe.Pointer(str("EDIT"))),
			uintptr(unsafe.Pointer(str(value))),
			flags,
			200, uintptr(yy), w-220, 22,
			hwnd, 0, hInst(), 0,
		)
		procSendMessage.Call(he, 0x0030, hFont, 1)
		s.fields[name] = he
		return he
	}

	rows := []struct {
		label, field, value string
		password            bool
	}{
		{"My Language (code):", "my_lang", initial.MyLang, false},
		{"Partner Language:", "partner_lang", initial.PartnerLang, false},
		{"AI Provider:", "ai_provider", initial.AIProvider, false},
		{"OpenAI Key:", "openai_key", initial.OpenAIKey, true},
		{"Claude Key:", "claude_key", initial.ClaudeKey, true},
		{"Gemini Key:", "gemini_key", initial.GeminiKey, true},
		{"Gemini Model:", "gemini_model", initial.GeminiModel, false},
		{"DeepL Key:", "deepl_key", initial.DeepLKey, true},
		{"Libre URL:", "libre_url", initial.LibreURL, false},
		{"Hotkey Translate In:", "hotkey_in", initial.HotkeyIn, false},
		{"Hotkey Translate Out:", "hotkey_out", initial.HotkeyOut, false},
	}

	for _, row := range rows {
		addLabel(row.label, y0)
		addEdit(row.field, row.value, y0, row.password)
		y0 += 30
	}

	// Lang hint
	langHint := "Language codes: vi, ko, en, ja, zh, fr, de, es, th"
	addLabel(langHint, y0)
	y0 += 24

	// Provider hint
	providerHint := "Providers: google_translate, openai, claude, gemini, deepl, libre"
	addLabel(providerHint, y0)
	y0 += 36

	// Status label for test
	hStatus, _, _ := procCreateWindowEx.Call(
		0, uintptr(unsafe.Pointer(str("STATIC"))), uintptr(unsafe.Pointer(str(""))),
		WS_CHILD|WS_VISIBLE|SS_LEFT,
		20, uintptr(y0), w-40, 18,
		hwnd, uintptr(801), hInst(), 0,
	)
	procSendMessage.Call(hStatus, 0x0030, hFontSmall, 1)
	s.fields["status"] = hStatus
	y0 += 30

	// Buttons
	btnY := h - 60
	makeBtn := func(label string, x, id int) {
		hb, _, _ := procCreateWindowEx.Call(
			0, uintptr(unsafe.Pointer(str("BUTTON"))),
			uintptr(unsafe.Pointer(str(label))),
			WS_CHILD|WS_VISIBLE|BS_PUSHBUTTON,
			uintptr(x), uintptr(btnY), 120, 30,
			hwnd, uintptr(id), hInst(), 0,
		)
		procSendMessage.Call(hb, 0x0030, hFont, 1)
	}
	makeBtn("Save", 20, 801)
	makeBtn("Test Provider", 160, 802)
	makeBtn("Close", 300, 803)

	procShowWindow.Call(hwnd, SW_SHOW)
	procUpdateWindow.Call(hwnd)

	var msg MSG
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), hwnd, 0, 0)
		if r == 0 || r == ^uintptr(0) {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
	globalSettings = nil
}

func getEditText(hwnd uintptr) string {
	buf := make([]uint16, 1024)
	procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), 1024)
	return windows.UTF16ToString(buf)
}

func settingsWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		if wParam>>16 == BN_CLICKED {
			id := wParam & 0xFFFF
			s := globalSettings
			if s == nil {
				break
			}
			switch id {
			case 801: // Save
				vals := s.readValues()
				if vals.MyLang == "" || vals.PartnerLang == "" {
					ShowMsg(hwnd, "Please enter language codes.", "Missing info", MB_ICONWARNING)
					return 0
				}
				s.onSave(vals)
				ShowMsg(hwnd, "Settings saved!", "Saved", MB_ICONINFORMATION)
				procDestroyWindow.Call(hwnd)
			case 802: // Test
				vals := s.readValues()
				procSetWindowText.Call(s.fields["status"], uintptr(unsafe.Pointer(str("Testing..."))))
				go func() {
					ok, msg := s.onTest(vals.AIProvider)
					prefix := "✓ "
					if !ok {
						prefix = "✗ "
					}
					procSetWindowText.Call(s.fields["status"], uintptr(unsafe.Pointer(str(prefix+msg))))
				}()
			case 803: // Close
				procDestroyWindow.Call(hwnd)
			}
		}
	case WM_CTLCOLORSTATIC:
		hdc := wParam
		procSetBkMode.Call(hdc, TRANSPARENT)
		procSetTextColor.Call(hdc, colorMuted)
		return brushBG
	case WM_CTLCOLOREDIT:
		hdc := wParam
		procSetTextColor.Call(hdc, colorText)
		return brushCard
	case WM_ERASEBKGND:
		hdc := wParam
		var rc RECT
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), brushBG)
		barRC := RECT{0, 0, rc.Right, 40}
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&barRC)), brushAccent)
		return 1
	case WM_CLOSE, WM_DESTROY:
		procDestroyWindow.Call(hwnd)
		globalSettings = nil
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
}

func (s *settingsState) readValues() SettingsValues {
	get := func(name string) string {
		h, ok := s.fields[name]
		if !ok {
			return ""
		}
		return strings.TrimSpace(getEditText(h))
	}
	return SettingsValues{
		MyLang:      get("my_lang"),
		PartnerLang: get("partner_lang"),
		AIProvider:  get("ai_provider"),
		OpenAIKey:   get("openai_key"),
		ClaudeKey:   get("claude_key"),
		GeminiKey:   get("gemini_key"),
		GeminiModel: get("gemini_model"),
		DeepLKey:    get("deepl_key"),
		LibreURL:    get("libre_url"),
		HotkeyIn:    get("hotkey_in"),
		HotkeyOut:   get("hotkey_out"),
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────────────

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

type MSG struct {
	HWND    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func ShowMsg(hwnd uintptr, text, title string, flags uint32) {
	procMessageBox.Call(hwnd, uintptr(unsafe.Pointer(str(text))), uintptr(unsafe.Pointer(str(title))), uintptr(flags))
}

// ShowInfo is a fire-and-forget toast popup
func ShowInfo(title, message string) {
	ShowPopup(title, "", message, 1800)
}

// ShowTranslation shows a translation result popup
func ShowTranslation(title, original, translated string) {
	ShowPopup(title, original, translated, 0)
}

// ControlWindow — main control panel
func ShowControlWindow(hotkeys string, onSettings func(), onQuit func()) {
	go func() {
		initResources()
		const w, h = 380, 220
		sw := screenW()
		sh := screenH()

		className := str("ITControl")
		wc := WNDCLASSEX{
			CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
			LpfnWndProc:   windows.NewCallback(controlWndProc),
			HInstance:     hInst(),
			HbrBackground: brushBG,
			LpszClassName: className,
		}
		procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

		hwnd, _, _ := procCreateWindowEx.Call(
			WS_EX_APPWINDOW,
			uintptr(unsafe.Pointer(className)),
			uintptr(unsafe.Pointer(str("Integrated Translator"))),
			WS_POPUP|WS_VISIBLE|WS_BORDER,
			uintptr((sw-w)/2), uintptr((sh-h)/2), w, h,
			0, 0, hInst(), 0,
		)

		_ = hwnd

		makeBtn := func(label string, x, y, id int) {
			hb, _, _ := procCreateWindowEx.Call(
				0, uintptr(unsafe.Pointer(str("BUTTON"))),
				uintptr(unsafe.Pointer(str(label))),
				WS_CHILD|WS_VISIBLE|BS_PUSHBUTTON,
				uintptr(x), uintptr(y), 200, 30,
				hwnd, uintptr(id), hInst(), 0,
			)
			procSendMessage.Call(hb, 0x0030, hFont, 1)
		}
		makeBtn("Open Settings", 90, 80, 901)
		makeBtn("Quit", 90, 122, 902)

		hk, _, _ := procCreateWindowEx.Call(
			0, uintptr(unsafe.Pointer(str("STATIC"))),
			uintptr(unsafe.Pointer(str(fmt.Sprintf("Hotkeys: %s", hotkeys)))),
			WS_CHILD|WS_VISIBLE|SS_CENTER,
			20, 50, w-40, 20,
			hwnd, 0, hInst(), 0,
		)
		procSendMessage.Call(hk, 0x0030, hFont, 1)

		// store callbacks in closure
		ctrlOnSettings = onSettings
		ctrlOnQuit = onQuit

		procShowWindow.Call(hwnd, SW_SHOW)
		procUpdateWindow.Call(hwnd)

		var msg MSG
		for {
			r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), hwnd, 0, 0)
			if r == 0 || r == ^uintptr(0) {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}()
}

var ctrlOnSettings func()
var ctrlOnQuit func()

func controlWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		if wParam>>16 == BN_CLICKED {
			switch wParam & 0xFFFF {
			case 901:
				if ctrlOnSettings != nil {
					go ctrlOnSettings()
				}
			case 902:
				if ctrlOnQuit != nil {
					go ctrlOnQuit()
				}
				procDestroyWindow.Call(hwnd)
			}
		}
	case WM_ERASEBKGND:
		hdc := wParam
		var rc RECT
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), brushBG)
		barRC := RECT{0, 0, rc.Right, 38}
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&barRC)), brushAccent)
		return 1
	case WM_CTLCOLORSTATIC:
		hdc := wParam
		procSetBkMode.Call(hdc, TRANSPARENT)
		procSetTextColor.Call(hdc, colorMuted)
		return brushBG
	case WM_CLOSE, WM_DESTROY:
		procDestroyWindow.Call(hwnd)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
}

// WaitMs sleeps without blocking the OS thread
func WaitMs(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}
