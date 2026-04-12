package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	FirstRun          bool     `json:"first_run"`
	MyLang            string   `json:"my_lang"`
	PartnerLang       string   `json:"partner_lang"`
	AutoDetect        bool     `json:"auto_detect"`
	HotkeyIn          string   `json:"hotkey_translate_in"`
	HotkeyOut         string   `json:"hotkey_translate_out"`
	AIProvider        string   `json:"ai_provider"`
	OpenAIKey         string   `json:"openai_api_key"`
	ClaudeKey         string   `json:"claude_api_key"`
	GeminiKey         string   `json:"gemini_api_key"`
	GeminiModel       string   `json:"gemini_model"`
	DeepLKey          string   `json:"deepl_api_key"`
	LibreURL          string   `json:"libre_url"`
	FallbackOrder     []string `json:"fallback_order"`
	AIEnhanced        bool     `json:"ai_enhanced"`
	FontSize          int      `json:"font_size"`
	PopupOpacity      float64  `json:"popup_opacity"`
	Theme             string   `json:"theme"`
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

func configPath() string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		home, _ := os.UserHomeDir()
		appData = filepath.Join(home, "AppData", "Roaming")
	}
	return filepath.Join(appData, "IntegratedTranslator", "config.json")
}

func Load() (*Config, error) {
	cfg := Defaults
	path := configPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &cfg, nil
		}
		return &cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &cfg, err
	}
	return &cfg, nil
}

func (c *Config) Save() error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
