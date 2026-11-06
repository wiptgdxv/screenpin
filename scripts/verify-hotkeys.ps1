param([string]$AppFile = 'artifacts/screenpin-check.exe')
$ErrorActionPreference = 'Stop'
$projectDir = Split-Path $PSScriptRoot -Parent
$artifactDir = Join-Path $projectDir 'artifacts'
New-Item -ItemType Directory -Force -Path $artifactDir | Out-Null
$env:APPDATA = Join-Path $env:TEMP ('screenpin-settings-test-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $env:APPDATA | Out-Null
$env:MYGO_LOCALE = 'zh-Hans'
$env:MYGO_AUTOMATION = Join-Path $env:TEMP ('screenpin-' + [guid]::NewGuid().ToString('N').Substring(0,8) + '.sock')
$process = Start-Process -FilePath (Join-Path $projectDir $AppFile) -WindowStyle Normal -PassThru -RedirectStandardError (Join-Path $artifactDir 'runtime.txt')
$sock = [System.Net.Sockets.Socket]::new([System.Net.Sockets.AddressFamily]::Unix,[System.Net.Sockets.SocketType]::Stream,[System.Net.Sockets.ProtocolType]::Unspecified)
$guard = [System.Diagnostics.Stopwatch]::StartNew()
while ($true) {
    try { $sock.Connect([System.Net.Sockets.UnixDomainSocketEndPoint]::new($env:MYGO_AUTOMATION)); break } catch {
        if ($process.HasExited) { throw "Application exited: $($process.ExitCode)" }
        if ($guard.Elapsed.TotalSeconds -gt 30) { throw }
        Start-Sleep -Milliseconds 50
    }
}
$stream = [System.Net.Sockets.NetworkStream]::new($sock,$true)
$writer = [System.IO.StreamWriter]::new($stream,[System.Text.UTF8Encoding]::new($false))
$writer.AutoFlush = $true
$reader = [System.IO.StreamReader]::new($stream)
$script:windowId = $null
function Invoke-App($method,$params=@{}) {
    if ($script:windowId -and $method -ne 'windows') { $params.window = $script:windowId }
    $writer.WriteLine((@{id=1;method=$method;params=$params} | ConvertTo-Json -Depth 8 -Compress))
    $res = $reader.ReadLine() | ConvertFrom-Json
    if ($res.error) { throw ($res.error | ConvertTo-Json -Compress) }
    return $res.result
}
while ($true) {
    $windows = @((Invoke-App 'windows').windows)
    if ($windows.Count -gt 0) { $script:windowId = $windows[0].id; break }
    if ($process.HasExited -or $guard.Elapsed.TotalSeconds -gt 30) { throw 'No window appeared' }
    Start-Sleep -Milliseconds 50
}
function Settle {
 Start-Sleep -Milliseconds 200
 Invoke-App 'capture' | Out-Null
}
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class ScreenpinTestFocus {
 [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr window);
 [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr window, int command);
}
'@
function Click-Label($label) {
    $process.Refresh()
    [ScreenpinTestFocus]::ShowWindow($process.MainWindowHandle,9) | Out-Null
    [ScreenpinTestFocus]::SetForegroundWindow($process.MainWindowHandle) | Out-Null
    Settle
    $node = (Invoke-App 'tree').nodes | Where-Object { $_.label -eq $label -and $_.actions -contains 'press' } | Select-Object -First 1
    if (!$node) { throw "Missing action: $label" }
    Invoke-App 'act' @{id=$node.id;action='press'} | Out-Null
    Settle
}
function Capture($name) {
    Settle
    $result = Invoke-App 'capture'
    [System.IO.File]::WriteAllBytes((Join-Path $artifactDir $name),[Convert]::FromBase64String($result.png))
}
Settle

try {
 Capture 'workspace-empty.png'
 Add-Type -AssemblyName System.Windows.Forms
 Add-Type -AssemblyName System.Drawing
 $testBitmap = [System.Drawing.Bitmap]::new(800,500)
 $testGraphics = [System.Drawing.Graphics]::FromImage($testBitmap)
 $testGraphics.Clear([System.Drawing.Color]::White)
 $testFont = [System.Drawing.Font]::new('Consolas',18)
 $testGraphics.DrawString('Clipboard image verification', $testFont, [System.Drawing.Brushes]::Black, 36, 32)
 $testGraphics.FillRectangle([System.Drawing.Brushes]::RoyalBlue,36,100,728,8)
 $testGraphics.DrawString('800 x 500 pixels', $testFont, [System.Drawing.Brushes]::Gray,36,145)
 [System.Windows.Forms.Clipboard]::SetImage($testBitmap)
 $testFont.Dispose(); $testGraphics.Dispose(); $testBitmap.Dispose()
 Invoke-App 'input' @{events=@(@{type='key';key='CmdOrCtrl+V'})} | Out-Null
 Settle
 Click-Label '放大'
 if ((Invoke-App 'tree').nodes.label -notcontains '115%') {throw 'Zoom did not update'}
 Click-Label '适应'
 Capture 'workspace-current.png'
 Click-Label '设置'
 Capture 'hotkey-settings.png'
 function Fill-Key($label,$value) {
  $node = (Invoke-App 'tree').nodes | Where-Object { $_.label -eq $label } | Select-Object -First 1
  if (!$node) { (Invoke-App 'tree') | ConvertTo-Json -Depth 10 | Set-Content (Join-Path $artifactDir 'hotkey-tree.json'); throw "Missing field: $label" }
  Invoke-App 'act' @{id=$node.id;action='setValue';text=$value} | Out-Null
  Settle
 }
 Fill-Key '剪贴板图片贴图快捷键' 'Ctrl+Alt+9'
 Click-Label '应用快捷键'
 $savedPath = Join-Path $env:APPDATA 'Screenpin/hotkeys.json'
 if (!(Test-Path -LiteralPath $savedPath)) {Capture 'hotkey-settings.png'; (Invoke-App 'tree') | ConvertTo-Json -Depth 10 | Set-Content (Join-Path $artifactDir 'hotkey-tree.json'); throw ('Apply failed: ' + ((Invoke-App 'tree').nodes.label -join ' / '))}
 $saved = Get-Content -LiteralPath $savedPath -Raw | ConvertFrom-Json
 if ($saved.pin -ne 'CmdOrCtrl+Alt+9') { throw 'Edited shortcut not persisted' }
 Fill-Key '显示面板快捷键' 'Ctrl+Alt+9'
 Click-Label '应用快捷键'
 if ((Invoke-App 'tree').nodes.label -notcontains '两个动作不能使用相同快捷键') {throw 'Duplicate shortcut not rejected'}
 $saved = Get-Content -LiteralPath $savedPath -Raw | ConvertFrom-Json
 if ($saved.show -eq 'CmdOrCtrl+Alt+9') {throw 'Duplicate modified saved config'}
 Click-Label '恢复默认快捷键'
 Click-Label '应用快捷键'
 $saved = Get-Content -LiteralPath $savedPath -Raw | ConvertFrom-Json
 if ($saved.pin -ne 'CmdOrCtrl+2') {throw 'Default shortcut not restored'}
 Capture 'hotkey-settings.png'
 Click-Label '深色外观'
 Click-Label '工作台'
 Click-Label '查看图片'
 Capture 'workspace-dark-current.png'
 Click-Label '设置'
 Fill-Key '剪贴板图片贴图快捷键' 'Ctrl+Alt+9'
 Click-Label '应用快捷键'
 Click-Label '工作台'
 Click-Label '查看图片'
 Click-Label '复制图片'
 $before = @((Invoke-App 'windows').windows).Count
 if (!('ScreenpinHotkeyTest' -as [type])) {
 Add-Type -TypeDefinition @'
using System.Runtime.InteropServices;
public static class ScreenpinHotkeyTest {
 [DllImport("user32.dll")] public static extern void keybd_event(byte key, byte scan, uint flags, System.UIntPtr extra);
}
'@
 }
 foreach ($keyCode in @(0x11,0x12,0x39)) { [ScreenpinHotkeyTest]::keybd_event($keyCode,0,0,[UIntPtr]::Zero) }
 foreach ($keyCode in @(0x39,0x12,0x11)) { [ScreenpinHotkeyTest]::keybd_event($keyCode,0,2,[UIntPtr]::Zero) }
 $triggerTimer = [Diagnostics.Stopwatch]::StartNew()
 while (@((Invoke-App 'windows').windows).Count -le $before) {
  if ($triggerTimer.Elapsed.TotalSeconds -gt 10) {throw 'Custom OS shortcut did not create a pin'}
  Start-Sleep -Milliseconds 100
 }
 Write-Output 'PASS: real Windows edit, persistence, custom OS hotkey trigger, duplicate rejection and defaults.'
} finally {
 $stream.Dispose()
 if (!$process.HasExited) {Stop-Process -Id $process.Id}
}
