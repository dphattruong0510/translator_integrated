package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"integrated-translator/internal/config"
)

var LangNames = map[string]string{
	"vi": "Vietnamese", "ko": "Korean", "en": "English",
	"ja": "Japanese", "zh": "Chinese", "fr": "French",
	"de": "German", "es": "Spanish", "th": "Thai",
	"auto": "Auto-detected language",
}

type Engine struct {
	Config        *config.Config
	quotaExceeded map[string]bool
	client        *http.Client
}

func New(cfg *config.Config) *Engine {
	return &Engine{
		Config:        cfg,
		quotaExceeded: make(map[string]bool),
		client:        &http.Client{Timeout: 15 * time.Second},
	}
}

func (e *Engine) Translate(text, srcLang, tgtLang string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	chain := e.buildChain(e.Config.AIProvider)
	for _, p := range chain {
		if e.quotaExceeded[p] {
			continue
		}
		result, err := e.callProvider(p, text, srcLang, tgtLang)
		if err != nil {
			if isQuotaError(err) {
				e.quotaExceeded[p] = true
				continue
			}
			continue
		}
		if result == "" {
			continue
		}
		// AI polish for google_translate results
		if e.Config.AIEnhanced && p == "google_translate" && e.hasAnyKey() {
			if polished := e.aiPolish(result, tgtLang); polished != "" {
				return polished
			}
		}
		return result
	}
	return "[Translation Error] Could not translate. Check internet or API key."
}

func (e *Engine) buildChain(primary string) []string {
	availableAI := []string{}
	checks := []struct{ provider, key string }{
		{"openai", e.Config.OpenAIKey},
		{"claude", e.Config.ClaudeKey},
		{"gemini", e.Config.GeminiKey},
		{"deepl", e.Config.DeepLKey},
		{"libre", e.Config.LibreURL},
	}
	for _, c := range checks {
		if strings.TrimSpace(c.key) != "" {
			availableAI = append(availableAI, c.provider)
		}
	}

	seen := map[string]bool{}
	chain := []string{}
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			chain = append(chain, p)
		}
	}

	if primary != "google_translate" {
		add(primary)
	} else {
		for _, p := range availableAI {
			add(p)
		}
	}
	for _, p := range e.Config.FallbackOrder {
		if p == "google_translate" && len(availableAI) > 0 {
			continue
		}
		add(p)
	}
	for _, p := range availableAI {
		add(p)
	}
	add("google_translate")
	return chain
}

func (e *Engine) hasAnyKey() bool {
	return e.Config.OpenAIKey != "" || e.Config.ClaudeKey != "" || e.Config.GeminiKey != ""
}

func (e *Engine) translationPrompt(text, src, tgt string) string {
	tgtName := LangNames[tgt]
	if tgtName == "" {
		tgtName = tgt
	}
	if src == tgt {
		return fmt.Sprintf("Make this %s text sound more natural and fluent. Keep the meaning. Return ONLY the improved text.\n\n%s", tgtName, text)
	}
	if src == "auto" || src == "" {
		return fmt.Sprintf("Detect the source language and translate to %s. Keep names, numbers, URLs accurate. Return ONLY the translated text.\n\n%s", tgtName, text)
	}
	srcName := LangNames[src]
	if srcName == "" {
		srcName = src
	}
	return fmt.Sprintf("Translate from %s to %s. Keep names, numbers, URLs accurate. Return ONLY the translated text.\n\n%s", srcName, tgtName, text)
}

func (e *Engine) callProvider(provider, text, src, tgt string) (string, error) {
	switch provider {
	case "google_translate":
		return e.googleTranslate(text, src, tgt)
	case "openai":
		if e.Config.OpenAIKey == "" {
			return "", fmt.Errorf("no OpenAI key")
		}
		return e.openAITranslate(text, src, tgt)
	case "claude":
		if e.Config.ClaudeKey == "" {
			return "", fmt.Errorf("no Claude key")
		}
		return e.claudeTranslate(text, src, tgt)
	case "gemini":
		if e.Config.GeminiKey == "" {
			return "", fmt.Errorf("no Gemini key")
		}
		return e.geminiTranslate(text, src, tgt)
	case "deepl":
		if e.Config.DeepLKey == "" {
			return "", fmt.Errorf("no DeepL key")
		}
		return e.deeplTranslate(text, src, tgt)
	case "libre":
		return e.libreTranslate(text, src, tgt)
	}
	return e.googleTranslate(text, src, tgt)
}

// ── Google Translate (free) ───────────────────────────────────────────────────

func (e *Engine) googleTranslate(text, src, tgt string) (string, error) {
	if src == "" {
		src = "auto"
	}
	apiURL := fmt.Sprintf(
		"https://translate.googleapis.com/translate_a/single?client=gtx&sl=%s&tl=%s&dt=t&q=%s",
		src, tgt, url.QueryEscape(text),
	)
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var data []interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("empty response")
	}
	parts, ok := data[0].([]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected format")
	}
	var sb strings.Builder
	for _, item := range parts {
		pair, ok := item.([]interface{})
		if ok && len(pair) > 0 {
			if s, ok := pair[0].(string); ok {
				sb.WriteString(s)
			}
		}
	}
	return sb.String(), nil
}

// ── OpenAI ───────────────────────────────────────────────────────────────────

func (e *Engine) openAITranslate(text, src, tgt string) (string, error) {
	payload := map[string]interface{}{
		"model": "gpt-3.5-turbo",
		"messages": []map[string]string{
			{"role": "user", "content": e.translationPrompt(text, src, tgt)},
		},
		"max_tokens":  1000,
		"temperature": 0.2,
	}
	return e.httpPost(
		"https://api.openai.com/v1/chat/completions",
		payload,
		map[string]string{"Authorization": "Bearer " + e.Config.OpenAIKey},
		func(d map[string]interface{}) (string, error) {
			choices := d["choices"].([]interface{})
			msg := choices[0].(map[string]interface{})["message"].(map[string]interface{})
			return strings.TrimSpace(msg["content"].(string)), nil
		},
	)
}

// ── Claude ───────────────────────────────────────────────────────────────────

func (e *Engine) claudeTranslate(text, src, tgt string) (string, error) {
	payload := map[string]interface{}{
		"model":      "claude-haiku-4-5-20251001",
		"max_tokens": 1000,
		"messages": []map[string]string{
			{"role": "user", "content": e.translationPrompt(text, src, tgt)},
		},
	}
	return e.httpPost(
		"https://api.anthropic.com/v1/messages",
		payload,
		map[string]string{
			"x-api-key":         e.Config.ClaudeKey,
			"anthropic-version": "2023-06-01",
		},
		func(d map[string]interface{}) (string, error) {
			content := d["content"].([]interface{})
			block := content[0].(map[string]interface{})
			return strings.TrimSpace(block["text"].(string)), nil
		},
	)
}

// ── Gemini ───────────────────────────────────────────────────────────────────

func (e *Engine) geminiTranslate(text, src, tgt string) (string, error) {
	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": []map[string]string{{"text": e.translationPrompt(text, src, tgt)}}},
		},
	}
	models := []string{}
	if m := strings.TrimSpace(e.Config.GeminiModel); m != "" {
		models = append(models, m)
	}
	for _, m := range []string{"gemini-2.5-flash", "gemini-2.0-flash", "gemini-2.5-flash-lite"} {
		models = append(models, m)
	}
	seen := map[string]bool{}
	var lastErr error
	for _, model := range models {
		if seen[model] {
			continue
		}
		seen[model] = true
		time.Sleep(1200 * time.Millisecond)
		apiURL := fmt.Sprintf(
			"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
			model, e.Config.GeminiKey,
		)
		result, err := e.httpPost(apiURL, payload, nil,
			func(d map[string]interface{}) (string, error) {
				candidates := d["candidates"].([]interface{})
				c := candidates[0].(map[string]interface{})
				content := c["content"].(map[string]interface{})
				parts := content["parts"].([]interface{})
				return strings.TrimSpace(parts[0].(map[string]interface{})["text"].(string)), nil
			},
		)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if isQuotaError(err) {
			time.Sleep(3 * time.Second)
		}
	}
	return "", lastErr
}

// ── DeepL ────────────────────────────────────────────────────────────────────

var deeplLangMap = map[string]string{
	"ko": "KO", "en": "EN", "ja": "JA", "zh": "ZH",
	"fr": "FR", "de": "DE", "es": "ES",
}

func (e *Engine) deeplTranslate(text, src, tgt string) (string, error) {
	tgtCode, ok := deeplLangMap[tgt]
	if !ok {
		return "", fmt.Errorf("DeepL does not support target lang: %s", tgt)
	}
	base := "https://api.deepl.com"
	if strings.HasSuffix(e.Config.DeepLKey, ":fx") {
		base = "https://api-free.deepl.com"
	}
	params := url.Values{}
	params.Set("text", text)
	params.Set("target_lang", tgtCode)
	if src != "" && src != "auto" {
		if srcCode, ok := deeplLangMap[src]; ok {
			params.Set("source_lang", srcCode)
		}
	}
	req, _ := http.NewRequest("POST", base+"/v2/translate", strings.NewReader(params.Encode()))
	req.Header.Set("Authorization", "DeepL-Auth-Key "+e.Config.DeepLKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", checkQuotaError(resp.StatusCode, string(body))
	}
	var d struct {
		Translations []struct{ Text string } `json:"translations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return "", err
	}
	if len(d.Translations) == 0 {
		return "", fmt.Errorf("empty DeepL response")
	}
	return d.Translations[0].Text, nil
}

// ── LibreTranslate ───────────────────────────────────────────────────────────

func (e *Engine) libreTranslate(text, src, tgt string) (string, error) {
	if src == "" {
		src = "auto"
	}
	payload := map[string]string{
		"q": text, "source": src, "target": tgt, "format": "text",
	}
	return e.httpPost(
		strings.TrimRight(e.Config.LibreURL, "/")+"/translate",
		payload, nil,
		func(d map[string]interface{}) (string, error) {
			return d["translatedText"].(string), nil
		},
	)
}

// ── AI Polish ────────────────────────────────────────────────────────────────

func (e *Engine) aiPolish(translated, tgt string) string {
	for _, p := range []string{"openai", "claude", "gemini"} {
		if e.quotaExceeded[p] {
			continue
		}
		result, err := e.callProvider(p, translated, tgt, tgt)
		if err == nil && result != "" {
			return result
		}
	}
	return ""
}

// ── Test Provider ────────────────────────────────────────────────────────────

func (e *Engine) TestProvider(provider string) (bool, string) {
	result, err := e.callProvider(provider, "Hello", "auto", "ko")
	if err != nil {
		if isQuotaError(err) {
			return false, "Quota exceeded / rate limited"
		}
		return false, fmt.Sprintf("Connection error: %s", err.Error())
	}
	if result == "" {
		return false, "No response returned"
	}
	return true, fmt.Sprintf("Active — Result: %s", result)
}

// ── HTTP helper ──────────────────────────────────────────────────────────────

func (e *Engine) httpPost(
	apiURL string,
	payload interface{},
	headers map[string]string,
	extractor func(map[string]interface{}) (string, error),
) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", checkQuotaError(resp.StatusCode, string(body))
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	return extractor(result)
}

type quotaError struct{ msg string }

func (q *quotaError) Error() string { return q.msg }

func checkQuotaError(code int, body string) error {
	lower := strings.ToLower(body)
	if code == 429 || code == 402 ||
		strings.Contains(lower, "quota") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "resource exhausted") ||
		strings.Contains(lower, "too many requests") {
		return &quotaError{fmt.Sprintf("HTTP %d: %s", code, body)}
	}
	return fmt.Errorf("HTTP %d: %s", code, body[:min(200, len(body))])
}

func isQuotaError(err error) bool {
	_, ok := err.(*quotaError)
	return ok
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
