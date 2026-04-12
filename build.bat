@echo off
setlocal EnableDelayedExpansion
chcp 65001 >nul

echo.
echo ================================================
echo   Integrated Translator — Go Build
echo   Ket qua: ~1-3 MB (sau UPX: ~500 KB - 1 MB)
echo ================================================
echo.

:: ── Check Go ────────────────────────────────────────
go version >nul 2>&1
if errorlevel 1 (
    echo [LOI] Khong tim thay Go.
    echo.
    echo Tai Go tai: https://go.dev/dl/
    echo Cai xong chay lai script nay.
    pause & exit /b 1
)
for /f "tokens=*" %%i in ('go version') do echo [OK] %%i

:: ── Download dependencies ───────────────────────────
echo.
echo [1/3] Tai dependencies...
go mod tidy
if errorlevel 1 ( echo [LOI] go mod tidy that bai. & pause & exit /b 1 )

:: ── Build ───────────────────────────────────────────
echo [2/3] Building...
echo.

:: -ldflags:
::   -s  = strip symbol table (giam ~30%%)
::   -w  = strip DWARF debug info (giam ~20%%)
::   -H windowsgui = khong hien cua so CMD
:: GOARCH=amd64 = 64-bit Windows
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64

go build ^
    -ldflags="-s -w -H windowsgui" ^
    -trimpath ^
    -o dist\IntegratedTranslator.exe ^
    ./cmd/translator

if errorlevel 1 (
    echo [LOI] Build that bai.
    pause & exit /b 1
)

:: ── UPX compress ────────────────────────────────────
echo [3/3] Nen file voi UPX...
where upx >nul 2>&1
if not errorlevel 1 (
    upx --best --lzma dist\IntegratedTranslator.exe
    echo [OK] Da nen xong.
) else (
    echo [INFO] UPX chua co. Tai: https://github.com/upx/upx/releases
    echo [INFO] Bo qua buoc nen, file van chay binh thuong.
)

:: ── Done ────────────────────────────────────────────
echo.
if exist dist\IntegratedTranslator.exe (
    for %%F in (dist\IntegratedTranslator.exe) do set SIZE=%%~zF
    set /a SIZE_KB=!SIZE! / 1024
    echo ================================================
    echo [THANH CONG]
    echo   File : dist\IntegratedTranslator.exe
    echo   Size : ~!SIZE_KB! KB
    echo   User chi can file .exe nay, khong can cai gi them.
    echo ================================================

    copy /Y dist\IntegratedTranslator.exe "%USERPROFILE%\Desktop\IntegratedTranslator.exe" >nul 2>&1
    echo [OK] Da copy len Desktop.
) else (
    echo [LOI] Khong tim thay file output.
)

echo.
pause
endlocal
