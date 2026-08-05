@echo off
setlocal
cd /d "%~dp0"

if not exist "python\python.exe" (
  echo [start] FATAL bundled python missing: %~dp0python\python.exe
  pause
  exit /b 2
)

echo [start] preparing portable environment ...
"python\python.exe" "bootstrap.py"
if errorlevel 2 (
  echo [start] bootstrap failed, aborting.
  pause
  exit /b 2
)

echo [start] launching ga-admin ...
start "" "ga-admin.exe"
exit /b 0
