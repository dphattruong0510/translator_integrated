package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	FirstRun      bool     `json:"first_run"`
	MyLang        string   `json:"my_lang"`
	PartnerLang   string   `json:"partner_lang"`
	AutoDetect    bool     `json:"auto_detect"`
	HotkeyIn      string   `json:"hotkey_translate_in"`
	HotkeyOut     string   `json:"hotkey_translate_out"`
	AIProvider    string   `json:"ai_provider"`
	OpenAIKey     string   `json:"openai_api_key"`
	ClaudeKey     string   `json:"claude_api_key"`
	GeminiKey     string   `json:"gemini_api_key"`
	GeminiModel   string   `json:"gemini_model"`
	DeepLKey      string   `json:"deepl_api_key"`
	LibreURL      string   `json:"libre_url"`
	FallbackOrder []string `json:"fallback_order"`
	AIEnhanced    bool     `json:"ai_enhanced"`
	FontSize      int      `json:"font_size"`
	PopupOpacity  float64  `json:"popup_opacity"`
	Theme         string   `json:"theme"`
}

var Defaults = Config{
	FirstRun:      true,
	MyLang:        "vi",
	PartnerLang:   "ko",
	AutoDetect:    true,
	HotkeyIn:      "ctrl+shift+r",
	HotkeyOut:     "ctrl+shift+w",
	AIProvider:    "google_translate",
	GeminiModel:   "gemini-2.5-flash",
	LibreURL:      "http://localhost:5000",
	FallbackOrder: []string{"openai", "claude", "gemini", "deepl", "google_translate"},
	AIEnhanced:    true,
	FontSize:      9,
	PopupOpacity:  0.92,
	Theme:         "dark",
}

func appDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(base, "IntegratedTranslatorGo")
}

func ConfigPath() string {
	return filepath.Join(appDir(), "config.json")
}

func Load() (*Config, error) {
	cfg := Defaults
	path := ConfigPath()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return &cfg, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if saveErr := cfg.Save(); saveErr != nil {
				return &cfg, saveErr
			}
			return &cfg, nil
		}
		return &cfg, err
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		cfg = Defaults
		if saveErr := cfg.Save(); saveErr != nil {
			return &cfg, saveErr
		}
		return &cfg, nil
	}

	sanitize(&cfg)
	if err := cfg.Save(); err != nil {
		return &cfg, err
	}
	return &cfg, nil
}

func (c *Config) Save() error {
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	sanitize(c)
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func sanitize(c *Config) {
	if c.MyLang == "" {
		c.MyLang = Defaults.MyLang
	}
	if c.PartnerLang == "" {
		c.PartnerLang = Defaults.PartnerLang
	}
	if c.AIProvider == "" {
		c.AIProvider = Defaults.AIProvider
	}
	if c.GeminiModel == "" {
		c.GeminiModel = Defaults.GeminiModel
	}
	if c.LibreURL == "" {
		c.LibreURL = Defaults.LibreURL
	}
	if len(c.FallbackOrder) == 0 {
		c.FallbackOrder = append([]string(nil), Defaults.FallbackOrder...)
	}
	if c.FontSize <= 0 {
		c.FontSize = Defaults.FontSize
	}
	if c.PopupOpacity <= 0 || c.PopupOpacity > 1 {
		c.PopupOpacity = Defaults.PopupOpacity
	}
	if c.Theme == "" {
		c.Theme = Defaults.Theme
	}
	c.HotkeyIn = normalizeHotkey(c.HotkeyIn, Defaults.HotkeyIn)
	c.HotkeyOut = normalizeHotkey(c.HotkeyOut, Defaults.HotkeyOut)
}

func normalizeHotkey(value, fallback string) string {
	v := strings.TrimSpace(strings.ToLower(value))
	if v == "" {
		return fallback
	}
	v = strings.NewReplacer(
		"<ctrl>", "ctrl",
		"<control>", "ctrl",
		"<shift>", "shift",
		"<alt>", "alt",
		"<cmd>", "cmd",
		"control", "ctrl",
		" ", "",
	).Replace(v)
	parts := strings.Split(v, "+")
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			clean = append(clean, p)
		}
	}
	if len(clean) < 2 {
		return fallback
	}
	return strings.Join(clean, "+")
}
