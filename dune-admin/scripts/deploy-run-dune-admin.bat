@echo off
setlocal

REM Double-click wrapper for the PowerShell deploy/run script.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0deploy-run-dune-admin.ps1" %*

endlocal
