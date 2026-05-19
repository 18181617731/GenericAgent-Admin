@echo off

setlocal

cd /d %~dp0

cd web

call npm.cmd install || exit /b 1

call npm.cmd run build || exit /b 1

cd ..

if not exist dist mkdir dist

go build -ldflags="-s -w -H=windowsgui" -o dist\ga-admin.exe . || exit /b 1

if not exist dist\cmd mkdir dist\cmd
copy /Y cmd\chat_worker.py dist\cmd\chat_worker.py >nul || exit /b 1

echo Built dist\ga-admin.exe
