@echo off
setlocal

cd /d %~dp0

for /f "usebackq delims=" %%i in (`git describe --tags --dirty --always 2^>nul`) do set "GA_VERSION=%%i"
if not defined GA_VERSION set "GA_VERSION=dev"

for /f "usebackq delims=" %%i in (`git rev-parse --short HEAD 2^>nul`) do set "GA_COMMIT=%%i"
if not defined GA_COMMIT set "GA_COMMIT=unknown"

for /f "usebackq delims=" %%i in (`powershell -NoProfile -Command "(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')"`) do set "GA_DATE=%%i"
if not defined GA_DATE set "GA_DATE=unknown"

set "GA_LDFLAGS=-s -w -H=windowsgui -X genericagent-admin-go/internal/version.Version=%GA_VERSION% -X genericagent-admin-go/internal/version.Commit=%GA_COMMIT% -X genericagent-admin-go/internal/version.Date=%GA_DATE%"

cd web
call npm.cmd install || exit /b 1
call npm.cmd run build || exit /b 1
cd ..

if not exist dist mkdir dist
go build -ldflags="%GA_LDFLAGS%" -o dist\ga-admin.exe . || exit /b 1

if not exist dist\cmd mkdir dist\cmd
copy /Y cmd\chat_worker.py dist\cmd\chat_worker.py >nul || exit /b 1

echo Built dist\ga-admin.exe
echo Version %GA_VERSION% Commit %GA_COMMIT% Date %GA_DATE%
