@echo off
setlocal

REM Double-click wrapper for the PowerShell deploy/run script.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0deploy-run-market-bot.ps1" %*

endlocal
