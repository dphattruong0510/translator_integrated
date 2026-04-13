//go:build windows

package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"integrated-translator/internal/config"
	"integrated-translator/internal/engine"
	"integrated-translator/internal/hotkey"
	"integrated-translator/internal/tray"
	"integrated-translator/internal/ui"
)

const singleInstancePort = 47651

// ── Single instance guard ─────────────────────────────────────────────────

func acquireSingleInstance() (net.Listener, bool) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", singleInstancePort))
	if err != nil {
		return nil, false
	}
	return ln, true
}

// ── App ───────────────────────────────────────────────────────────────────

type App struct {
	cfg          *config.Config
	eng          *engine.Engine
	hkMgr        *hotkey.Manager
	trayIcon     *tray.Icon
	selLock      sync.Mutex
	hkInID       int32
	hkOutID      int32
}

func NewApp(cfg *config.Config) *App {
	return &App{
		cfg:   cfg,
		eng:   engine.New(cfg),
		hkMgr: hotkey.NewManager(),
	}
}

func (a *App) langName(code string) string {
	return ui.LangCodeToName[code]
}

// GetSelectedText — saves clipboard, sends Ctrl+C, reads result
func (a *App) GetSelectedText() string {
	a.selLock.Lock()
	defer a.selLock.Unlock()

	original := hotkey.GetClipboard()
	sentinel := fmt.Sprintf("__it_probe_%d__", time.Now().UnixNano())
	hotkey.SetClipboard(sentinel)
	time.Sleep(40 * time.Millisecond)

	var result string
	for i := 0; i < 4; i++ {
		hotkey.SendCtrlC()
		time.Sleep(160 * time.Millisecond)
		cur := hotkey.GetClipboard()
		if cur != sentinel && cur != "" {
			result = cur
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	hotkey.SetClipboard(original)
	return strings.TrimSpace(result)
}

// ReplaceSelectedText — sets clipboard and pastes
func (a *App) ReplaceSelectedText(text string) {
	a.selLock.Lock()
	defer a.selLock.Unlock()

	original := hotkey.GetClipboard()
	hotkey.SetClipboard(text)
	time.Sleep(60 * time.Millisecond)
	hotkey.SendCtrlV()
	time.Sleep(100 * time.Millisecond)
	hotkey.SetClipboard(original)
}

// ── Translate In (receive) ────────────────────────────────────────────────

func (a *App) HandleTranslateIn() {
	text := a.GetSelectedText()
	if text == "" {
		ui.ShowInfo("No text", "Select some text first.")
		return
	}

	srcLang := a.cfg.PartnerLang
	tgtLang := a.cfg.MyLang
	if a.cfg.AutoDetect {
		srcLang = "auto"
	}

	ui.ShowInfo("Translating…", fmt.Sprintf("→ %s", a.langName(tgtLang)))
	result := a.eng.Translate(text, srcLang, tgtLang)
	ui.ShowTranslation(
		fmt.Sprintf("→ %s", a.langName(tgtLang)),
		text, result,
	)
}

// ── Translate Out (send) ──────────────────────────────────────────────────

func (a *App) HandleTranslateOut() {
	text := a.GetSelectedText()
	if text == "" {
		ui.ShowInfo("No text", "Select some text first.")
		return
	}

	srcLang := a.cfg.MyLang
	tgtLang := a.cfg.PartnerLang
	if a.cfg.AutoDetect {
		srcLang = "auto"
	}

	ui.ShowInfo("Translating…", fmt.Sprintf("→ %s", a.langName(tgtLang)))
	result := a.eng.Translate(text, srcLang, tgtLang)

	combined := fmt.Sprintf("%s / %s", text, result)
	a.ReplaceSelectedText(combined)
	ui.ShowInfo("Replaced", fmt.Sprintf("Inserted as: original / %s", a.langName(tgtLang)))
}

// ── Hotkeys ───────────────────────────────────────────────────────────────

func (a *App) SetupHotkeys() error {
	inID, err := a.hkMgr.Register(a.cfg.HotkeyIn, func() {
		go a.HandleTranslateIn()
	})
	if err != nil {
		return fmt.Errorf("hotkey in: %w", err)
	}
	a.hkInID = inID

	outID, err := a.hkMgr.Register(a.cfg.HotkeyOut, func() {
		go a.HandleTranslateOut()
	})
	if err != nil {
		return fmt.Errorf("hotkey out: %w", err)
	}
	a.hkOutID = outID
	return nil
}

func (a *App) ReloadHotkeys() {
	if a.hkInID != 0 {
		a.hkMgr.Unregister(a.hkInID)
	}
	if a.hkOutID != 0 {
		a.hkMgr.Unregister(a.hkOutID)
	}
	a.SetupHotkeys()
}

// ── Tray ─────────────────────────────────────────────────────────────────

func (a *App) SetupTray() {
	hkLabel := fmt.Sprintf("%s | %s", a.cfg.HotkeyIn, a.cfg.HotkeyOut)
	items := []tray.MenuItem{
		{Label: "Integrated Translator", ID: 0},
		{Separator: true},
		{Label: fmt.Sprintf("Translate In  (%s)", a.cfg.HotkeyIn), ID: 1, Action: func() { go a.HandleTranslateIn() }},
		{Label: fmt.Sprintf("Translate Out (%s)", a.cfg.HotkeyOut), ID: 2, Action: func() { go a.HandleTranslateOut() }},
		{Separator: true},
		{Label: "Settings", ID: 3, Action: func() { a.OpenSettings() }},
		{Label: "Quit", ID: 4, Action: func() { a.Quit() }},
	}
	_ = hkLabel
	a.trayIcon = tray.New("Integrated Translator", items, func() { a.OpenSettings() })
}

// ── Settings ──────────────────────────────────────────────────────────────

func (a *App) OpenSettings() {
	initial := ui.SettingsValues{
		MyLang:      a.cfg.MyLang,
		PartnerLang: a.cfg.PartnerLang,
		AutoDetect:  a.cfg.AutoDetect,
		AIEnhanced:  a.cfg.AIEnhanced,
		AIProvider:  a.cfg.AIProvider,
		OpenAIKey:   a.cfg.OpenAIKey,
		ClaudeKey:   a.cfg.ClaudeKey,
		GeminiKey:   a.cfg.GeminiKey,
		GeminiModel: a.cfg.GeminiModel,
		DeepLKey:    a.cfg.DeepLKey,
		LibreURL:    a.cfg.LibreURL,
		HotkeyIn:    a.cfg.HotkeyIn,
		HotkeyOut:   a.cfg.HotkeyOut,
	}
	ui.ShowSettings(initial,
		func(vals ui.SettingsValues) {
			a.cfg.MyLang = vals.MyLang
			a.cfg.PartnerLang = vals.PartnerLang
			a.cfg.AutoDetect = vals.AutoDetect
			a.cfg.AIEnhanced = vals.AIEnhanced
			a.cfg.AIProvider = vals.AIProvider
			a.cfg.OpenAIKey = vals.OpenAIKey
			a.cfg.ClaudeKey = vals.ClaudeKey
			a.cfg.GeminiKey = vals.GeminiKey
			a.cfg.GeminiModel = vals.GeminiModel
			a.cfg.DeepLKey = vals.DeepLKey
			a.cfg.LibreURL = vals.LibreURL
			a.cfg.HotkeyIn = vals.HotkeyIn
			a.cfg.HotkeyOut = vals.HotkeyOut
			a.cfg.Save()
			a.eng = engine.New(a.cfg)
			a.ReloadHotkeys()
		},
		func(provider string) (bool, string) {
			return a.eng.TestProvider(provider)
		},
	)
}

// ── Quit ─────────────────────────────────────────────────────────────────

func (a *App) Quit() {
	if a.trayIcon != nil {
		a.trayIcon.Stop()
	}
	if a.hkInID != 0 {
		a.hkMgr.Unregister(a.hkInID)
	}
	if a.hkOutID != 0 {
		a.hkMgr.Unregister(a.hkOutID)
	}
	os.Exit(0)
}

// ── Main ──────────────────────────────────────────────────────────────────

func main() {
	ln, ok := acquireSingleInstance()
	if !ok {
		ui.ShowMsg(0, "Integrated Translator is already running.\nCheck the system tray.", "Already Running", 0x40)
		return
	}
	defer ln.Close()

	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Defaults
		_ = cfg.Save()
	}

	app := NewApp(cfg)

	if err := app.SetupHotkeys(); err != nil {
		ui.ShowMsg(0, fmt.Sprintf("Could not register hotkeys: %v\nTry changing the hotkeys in Settings.", err), "Hotkey Error", 0x30)
	}

	app.SetupTray()

	if cfg.FirstRun {
		cfg.FirstRun = false
		cfg.Save()
		app.OpenSettings()
	} else {
		ui.ShowInfo("Integrated Translator", fmt.Sprintf(
			"Running in tray.\n%s → translate in\n%s → translate out",
			cfg.HotkeyIn, cfg.HotkeyOut,
		))
	}

	// Block on tray message pump (runs the system tray)
	app.trayIcon.Run()
}
