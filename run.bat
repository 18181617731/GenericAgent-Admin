@echo off
setlocal EnableExtensions

cd /d "%~dp0"

echo GenericAgent Admin - build and run
echo.

call :stop_running_admin || (
  echo.
  echo ERROR: Failed to stop the previous GA Admin instance.
  pause
  exit /b 1
)

call "%~dp0build.bat"
if errorlevel 1 (
  echo.
  echo Build failed. Review the error above, then run this file again.
  pause
  exit /b 1
)

if not exist "%~dp0dist\ga-admin.exe" (
  echo.
  echo ERROR: dist\ga-admin.exe was not created.
  pause
  exit /b 1
)

echo.
echo Starting GenericAgent Admin...
start "" /D "%~dp0dist" "%~dp0dist\ga-admin.exe" --port 8787
if errorlevel 1 (
  echo ERROR: Failed to start dist\ga-admin.exe.
  pause
  exit /b 1
)

exit /b 0

:stop_running_admin
if not exist "%~dp0dist\ga-admin.exe" exit /b 0

echo [0/3] Checking for a previous GA Admin instance...
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$target=[IO.Path]::GetFullPath('%~dp0dist\ga-admin.exe'); $running=@(Get-CimInstance Win32_Process -Filter 'Name=''ga-admin.exe''' | Where-Object { $_.ExecutablePath -and [IO.Path]::GetFullPath($_.ExecutablePath) -eq $target }); foreach ($process in $running) { Write-Host ('Stopping PID ' + $process.ProcessId + ' so the application can be updated...'); Stop-Process -Id $process.ProcessId -Force -ErrorAction Stop }; if ($running.Count -gt 0) { Start-Sleep -Milliseconds 500 }"
exit /b %errorlevel%
