# Integrated Translator — Go Version

## Tại sao rewrite bằng Go?

| | Python + PyInstaller | Go (bản này) |
|---|---|---|
| **Kích thước** | 25–35 MB | **1–3 MB** |
| **Sau UPX** | 15–22 MB | **~500 KB – 1 MB** |
| **Cần cài thêm** | Không | Không |
| **Startup** | 1–3 giây | < 0.1 giây |
| **RAM** | ~60–100 MB | ~5–15 MB |
| **Dependencies** | pynput, pystray, pillow, pyperclip | `golang.org/x/sys` (1 package) |

Go compile thẳng ra native Windows binary — không cần runtime, không cần Python, không có overhead.

---

## Cách build

### Yêu cầu
- [Go 1.21+](https://go.dev/dl/) — cài một lần, ~70 MB
- (Tùy chọn) [UPX](https://github.com/upx/upx/releases) để nén thêm

### Build
```
build.bat
```

Xong. File `dist/IntegratedTranslator.exe` là tất cả những gì user cần.

---

## Cấu trúc project

```
gotranslator/
├── cmd/translator/
│   ├── main.go          ← Entry point, wires everything
│   └── app.manifest     ← Windows DPI + permissions manifest
├── internal/
│   ├── config/
│   │   └── config.go    ← Đọc/ghi config.json
│   ├── engine/
│   │   └── engine.go    ← Multi-provider translation (Google/OpenAI/Claude/Gemini/DeepL/Libre)
│   ├── hotkey/
│   │   └── hotkey_windows.go  ← RegisterHotKey Win32 API + clipboard
│   ├── tray/
│   │   └── tray_windows.go    ← Shell_NotifyIcon (system tray)
│   └── ui/
│       └── ui_windows.go      ← Popup + Settings + Control window (Win32 native)
├── go.mod
├── go.sum
└── build.bat
```

**Không dùng framework UI nào cả** — tất cả UI dùng trực tiếp Win32 API qua `golang.org/x/sys/windows`.

---

## Features giữ nguyên so với bản Python

- ✅ Global hotkeys (Ctrl+Shift+R / Ctrl+Shift+W)
- ✅ System tray icon với context menu
- ✅ Popup dịch floating, dark theme
- ✅ Get selected text (Ctrl+C) + replace (Ctrl+V)
- ✅ Multi-provider: Google Translate, OpenAI, Claude, Gemini, DeepL, LibreTranslate
- ✅ Fallback chain khi provider lỗi/quota hết
- ✅ AI Polish (Google → AI refine)
- ✅ Settings window với test provider
- ✅ Config lưu tại `%APPDATA%\IntegratedTranslator\config.json`
- ✅ Single instance guard

---

## Config location

```
C:\Users\<tên>\AppData\Roaming\IntegratedTranslator\config.json
```

Config tương thích hoàn toàn với bản Python cũ — user giữ nguyên API keys, không cần config lại.

---

## Phân phối cho user

Gửi duy nhất 1 file:
```
IntegratedTranslator.exe  (~500 KB – 3 MB)
```

User double-click là chạy ngay. Không cần installer, không cần Python.
