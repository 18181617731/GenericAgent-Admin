@echo off
setlocal EnableExtensions

cd /d "%~dp0"

echo GenericAgent Admin - build and run
echo.

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
start "" /D "%~dp0dist" "%~dp0dist\ga-admin.exe"
if errorlevel 1 (
  echo ERROR: Failed to start dist\ga-admin.exe.
  pause
  exit /b 1
)

exit /b 0
