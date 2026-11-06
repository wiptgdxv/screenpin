param([string]$AppFile = 'artifacts/screenpin-check.exe')
# The current desktop suite includes empty and loaded layouts, zoom, settings,
# appearance, clipboard, and a native custom global shortcut creating a pin.
& (Join-Path $PSScriptRoot 'verify-hotkeys.ps1') -AppFile $AppFile
