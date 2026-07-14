@echo off
setlocal EnableExtensions

cd /d "%~dp0"

call :resolve_npm || exit /b 1
call :resolve_go || exit /b 1
call :configure_go_proxy || exit /b 1

for /f "usebackq delims=" %%i in (`git describe --tags --dirty --always 2^>nul`) do set "GA_VERSION=%%i"
if not defined GA_VERSION set "GA_VERSION=dev"

for /f "usebackq delims=" %%i in (`git rev-parse --short HEAD 2^>nul`) do set "GA_COMMIT=%%i"
if not defined GA_COMMIT set "GA_COMMIT=unknown"

for /f "usebackq delims=" %%i in (`powershell -NoProfile -Command "(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')"`) do set "GA_DATE=%%i"
if not defined GA_DATE set "GA_DATE=unknown"

set "GA_LDFLAGS=-s -w -H=windowsgui -X genericagent-admin-go/internal/version.Version=%GA_VERSION% -X genericagent-admin-go/internal/version.Commit=%GA_COMMIT% -X genericagent-admin-go/internal/version.Date=%GA_DATE%"

call :ensure_frontend_dependencies || exit /b 1

echo [2/3] Building embedded frontend...
call "%NPM_EXE%" --prefix web run build
if errorlevel 1 (
  echo ERROR: Frontend build failed.
  exit /b 1
)

if not exist dist mkdir dist
echo [3/3] Building Go application with "%GO_EXE%"...
"%GO_EXE%" build -ldflags="%GA_LDFLAGS%" -o dist\ga-admin.exe .
if errorlevel 1 (
  echo ERROR: Go build failed.
  exit /b 1
)

if not exist dist\cmd mkdir dist\cmd
copy /Y cmd\chat_worker.py dist\cmd\chat_worker.py >nul || exit /b 1

echo Built dist\ga-admin.exe
echo Version %GA_VERSION% Commit %GA_COMMIT% Date %GA_DATE%
exit /b 0

:ensure_frontend_dependencies
set "NPM_STAMP=web\node_modules\.ga-admin-package-lock.sha256"
set "LOCK_HASH="
for /f "usebackq delims=" %%i in (`powershell -NoProfile -Command "$sha=[Security.Cryptography.SHA256]::Create(); try { [BitConverter]::ToString($sha.ComputeHash([IO.File]::ReadAllBytes('web\package-lock.json'))).Replace('-','') } finally { $sha.Dispose() }"`) do set "LOCK_HASH=%%i"
if not defined LOCK_HASH (
  echo ERROR: Failed to hash web\package-lock.json.
  exit /b 1
)

set "INSTALLED_HASH="
if exist "%NPM_STAMP%" set /p INSTALLED_HASH=<"%NPM_STAMP%"
if exist web\node_modules if /I "%INSTALLED_HASH%"=="%LOCK_HASH%" (
  echo [1/3] Frontend dependencies are up to date; skipping npm ci.
  exit /b 0
)

echo [1/3] Installing frontend dependencies with npm ci...
call "%NPM_EXE%" --prefix web ci
if errorlevel 1 (
  echo ERROR: Frontend dependency installation failed.
  exit /b 1
)
>"%NPM_STAMP%" echo %LOCK_HASH%
exit /b 0

:resolve_npm
set "NPM_EXE="
for /f "delims=" %%i in ('where npm.cmd 2^>nul') do if not defined NPM_EXE set "NPM_EXE=%%i"
if not defined NPM_EXE if exist "%ProgramFiles%\nodejs\npm.cmd" set "NPM_EXE=%ProgramFiles%\nodejs\npm.cmd"
if not defined NPM_EXE (
  echo ERROR: npm.cmd was not found. Install Node.js 22 or add it to PATH.
  exit /b 1
)
exit /b 0

:resolve_go
set "GO_EXE="
for /f "delims=" %%i in ('where go.exe 2^>nul') do if not defined GO_EXE set "GO_EXE=%%i"
if not defined GO_EXE if exist "%ProgramFiles%\Go\bin\go.exe" set "GO_EXE=%ProgramFiles%\Go\bin\go.exe"
if not defined GO_EXE if exist "%LOCALAPPDATA%\Programs\Go\bin\go.exe" set "GO_EXE=%LOCALAPPDATA%\Programs\Go\bin\go.exe"
if not defined GO_EXE if exist "%SystemDrive%\Go\bin\go.exe" set "GO_EXE=%SystemDrive%\Go\bin\go.exe"
if not defined GO_EXE (
  echo Go was not found; downloading a verified portable toolchain...
  powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\bootstrap-go.ps1" -InstallDir "%~dp0.tools\go" -MinimumVersion "1.23"
  if errorlevel 1 (
    echo ERROR: Unable to prepare Go 1.23 or newer automatically.
    exit /b 1
  )
  if exist "%~dp0.tools\go\bin\go.exe" set "GO_EXE=%~dp0.tools\go\bin\go.exe"
)
if not defined GO_EXE (
  echo ERROR: go.exe was not found after automatic setup.
  exit /b 1
)
exit /b 0

:configure_go_proxy
if defined GOPROXY (
  echo Using GOPROXY from the current environment: %GOPROXY%
  exit /b 0
)

set "CONFIGURED_GOPROXY="
for /f "usebackq delims=" %%i in (`"%GO_EXE%" env GOPROXY 2^>nul`) do if not defined CONFIGURED_GOPROXY set "CONFIGURED_GOPROXY=%%i"
if /I "%CONFIGURED_GOPROXY%"=="https://proxy.golang.org,direct" (
  set "GOPROXY=https://goproxy.cn,direct"
  echo The default Go module proxy is replaced with https://goproxy.cn for this build.
) else if defined CONFIGURED_GOPROXY (
  echo Using GOPROXY from go env: %CONFIGURED_GOPROXY%
)
exit /b 0
