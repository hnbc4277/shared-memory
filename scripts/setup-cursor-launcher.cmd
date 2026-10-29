@echo off
cd /d "%~dp0"
set "PS1=%~dpn0.ps1"
if not exist "%PS1%" (
  echo ERROR: missing "%PS1%"
  echo Keep the .cmd and .ps1 in the same folder with the same base name.
  pause
  exit /b 1
)
echo Shared Memory Cursor setup
echo.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%PS1%"
set ERR=%ERRORLEVEL%
echo.
if not "%ERR%"=="0" (
  echo Setup failed, exit code %ERR%
  pause
  exit /b %ERR%
)
echo Done. Fully quit Cursor, reopen it, then send a test message.
pause
