param([string]$AppFile = 'artifacts/screenpin-check.exe', [switch]$AppearanceOnly)
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
 [DllImport("user32.dll")] public static extern bool SetCursorPos(int x,int y);
 [DllImport("user32.dll")] public static extern void mouse_event(uint flags,int x,int y,int data,UIntPtr extra);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern IntPtr FindWindow(string className,string title);
 [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr window);
 [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr window, int command);
 [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr window, out NativeRect rectangle);
 [DllImport("dwmapi.dll")] public static extern int DwmGetWindowAttribute(IntPtr window,int attribute,out int value,int size);
 [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr window,IntPtr after,int x,int y,int cx,int cy,uint flags);
 public struct NativeRect { public int Left, Top, Right, Bottom; }
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
 Add-Type -AssemblyName System.Windows.Forms
 Add-Type -AssemblyName System.Drawing
 function Save-DesktopWindow($name, [IntPtr]$handle=[IntPtr]::Zero) {
  if ($handle -eq [IntPtr]::Zero) {
   $process.Refresh()
   $handle=$process.MainWindowHandle
  }
  [ScreenpinTestFocus]::ShowWindow($handle,9) | Out-Null
  [ScreenpinTestFocus]::SetForegroundWindow($handle) | Out-Null
  Start-Sleep -Milliseconds 500
  $windowRect = [ScreenpinTestFocus+NativeRect]::new()
  [ScreenpinTestFocus]::GetWindowRect($handle,[ref]$windowRect) | Out-Null
  $shot = [System.Drawing.Bitmap]::new($windowRect.Right-$windowRect.Left,$windowRect.Bottom-$windowRect.Top)
  $graphics = [System.Drawing.Graphics]::FromImage($shot)
  try {
   $graphics.CopyFromScreen($windowRect.Left,$windowRect.Top,0,0,$shot.Size)
   $shot.Save((Join-Path $artifactDir $name),[System.Drawing.Imaging.ImageFormat]::Png)
  } finally {$graphics.Dispose();$shot.Dispose()}
 }
 function Assert-Backdrop($frosted) {
  $snapshot=Invoke-App 'capture'
  $bytes=[Convert]::FromBase64String($snapshot.png)
  $memory=[System.IO.MemoryStream]::new($bytes)
  $bitmap=[System.Drawing.Bitmap]::new($memory)
  try {
   foreach ($fraction in @(0.02,0.10,0.45,0.98)) {
    $alpha=$bitmap.GetPixel([int]($bitmap.Width*0.55),[int]($bitmap.Height*$fraction)).A
    if ($frosted -and $alpha -eq 255) {throw "Opaque surface at $fraction hides native backdrop"}
    if (!$frosted -and $alpha -ne 255) {throw "Transparent surface at $fraction in solid mode"}
   }
  } finally {$bitmap.Dispose();$memory.Dispose()}
 }
 Assert-Backdrop $true
 Click-Label '设置'
 Assert-Backdrop $true
 Capture 'settings-glass-current.png'
 Click-Label '常规'
 Capture 'settings-general-current.png'
 Click-Label '快捷键'
 Capture 'settings-hotkeys-current.png'
 Click-Label '工作台'
 Click-Label '设置'
 Click-Label '窗口外观'
 Save-DesktopWindow 'settings-glass-desktop.png'
 Click-Label '工作台'
 Assert-Backdrop $true
 Capture 'workspace-glass-empty.png'
 if (!$AppearanceOnly) {
 $testBitmap=[System.Drawing.Bitmap]::new(800,500)
 $testGraphics=[System.Drawing.Graphics]::FromImage($testBitmap)
 $testGraphics.Clear([System.Drawing.Color]::FromArgb(37,65,83))
 $testGraphics.FillRectangle([System.Drawing.Brushes]::RoyalBlue,36,100,728,8)
 [System.Windows.Forms.Clipboard]::SetImage($testBitmap)
 $testGraphics.Dispose();$testBitmap.Dispose()
 Invoke-App 'input' @{events=@(@{type='key';key='CmdOrCtrl+V'})} | Out-Null
 Settle
 Click-Label '工作台'
 Click-Label '查看图片'
 Capture 'workspace-glass-current.png'
 Click-Label '贴到桌面'
 $allWindows=@((Invoke-App 'windows').windows)
 $pinWindow=$allWindows | Where-Object {$_.title -like '贴图*'} | Select-Object -First 1
 $mainWindow=$allWindows | Where-Object {$_.title -like '*控制面板*'} | Select-Object -First 1
 if (!$pinWindow) {throw 'No pin created'}
 $pinHandle=[ScreenpinTestFocus]::FindWindow([NullString]::Value,$pinWindow.title)
 Save-DesktopWindow 'pin-border-desktop.png' $pinHandle
 $pinShot=[System.Drawing.Bitmap]::new((Join-Path $artifactDir 'pin-border-desktop.png'))
 try {
  $edge=$pinShot.GetPixel([int]($pinShot.Width/2),0)
  if ($edge.R -gt 110 -or $edge.G -gt 130 -or $edge.B -gt 155) {throw "White border remains on pin ($edge)"}
 } finally {$pinShot.Dispose()}
 $script:windowId=$pinWindow.id
 Capture 'pin-glass-current.png'
 $beforeZoom=[ScreenpinTestFocus+NativeRect]::new()
 [ScreenpinTestFocus]::GetWindowRect($pinHandle,[ref]$beforeZoom) | Out-Null
 [ScreenpinTestFocus]::SetCursorPos([int](($beforeZoom.Left+$beforeZoom.Right)/2),[int](($beforeZoom.Top+$beforeZoom.Bottom)/2)) | Out-Null
 Start-Sleep -Milliseconds 100
 [ScreenpinTestFocus]::mouse_event(0x0800,0,0,120,[UIntPtr]::Zero)
 Settle
 $afterZoom=[ScreenpinTestFocus+NativeRect]::new()
 [ScreenpinTestFocus]::GetWindowRect($pinHandle,[ref]$afterZoom) | Out-Null
 if ($afterZoom.Right -ne $beforeZoom.Right -or $afterZoom.Bottom -ne $beforeZoom.Bottom) {throw 'Wheel zoom changed the pin window size'}
 if ((Invoke-App 'tree').nodes.label -notcontains '115%') {throw 'Pin did not show its enlarged percentage'}
 [ScreenpinTestFocus]::mouse_event(0x0800,0,0,-120,[UIntPtr]::Zero)
 Settle
 [ScreenpinTestFocus]::GetWindowRect($pinHandle,[ref]$afterZoom) | Out-Null
 if ([Math]::Abs(($afterZoom.Right-$afterZoom.Left)-($beforeZoom.Right-$beforeZoom.Left)) -gt 2) {throw 'Wheel zoom changed the pin window size'}
 if ((Invoke-App 'tree').nodes.label -notcontains '100%') {throw 'Pin did not show 100 percent after shrinking'}
 Start-Sleep -Milliseconds 1350
 if ((Invoke-App 'tree').nodes.label -contains '100%') {throw 'Pin zoom label did not disappear'}
 [ScreenpinTestFocus]::mouse_event(0x0800,0,0,120,[UIntPtr]::Zero)
 Settle
 if ((Invoke-App 'tree').nodes.label -notcontains '115%') {throw 'Pin did not zoom before resizing'}
 $edgeY=[int](($beforeZoom.Top+$beforeZoom.Bottom)/2)
 [ScreenpinTestFocus]::SetCursorPos($beforeZoom.Right-4,$edgeY) | Out-Null
 Start-Sleep -Milliseconds 100
 [ScreenpinTestFocus]::mouse_event(0x0002,0,0,0,[UIntPtr]::Zero)
 try {
  Start-Sleep -Milliseconds 100
  [ScreenpinTestFocus]::SetCursorPos($beforeZoom.Right-124,$edgeY) | Out-Null
  Start-Sleep -Milliseconds 250
 } finally { [ScreenpinTestFocus]::mouse_event(0x0004,0,0,0,[UIntPtr]::Zero) }
 Settle
 [ScreenpinTestFocus]::GetWindowRect($pinHandle,[ref]$afterZoom) | Out-Null
 if (($afterZoom.Right-$afterZoom.Left) -ge ($beforeZoom.Right-$beforeZoom.Left)-60) {throw 'Dragging the pin edge did not narrow the window'}
 $resizeScale=(Invoke-App 'tree').nodes.label | Where-Object {$_ -match '^\d+%$'} | Select-Object -First 1
 if (!$resizeScale -or [int]$resizeScale.TrimEnd('%') -ge 110) {throw "Image zoom did not follow the window resize ($resizeScale)"}


 $script:windowId=$mainWindow.id
 }
 Click-Label '设置'
 Click-Label '毛玻璃效果'
 Assert-Backdrop ([Environment]::OSVersion.Version.Build -ge 22621)
 Click-Label '毛玻璃效果'
 Assert-Backdrop $true
 function Set-AppearanceSlider($label,$field,$value,$action,$count) {
  $slider=(Invoke-App 'tree').nodes | Where-Object {$_.label -eq $label -and $_.actions -contains 'increment'} | Select-Object -First 1
  if (!$slider) {throw "Missing appearance slider $label"}
  for ($step=0;$step -lt $count;$step++) {Invoke-App 'act' @{id=$slider.id;action=$action} | Out-Null; Start-Sleep -Milliseconds 22}
  Settle
  $settings=Get-Content -LiteralPath (Join-Path $env:APPDATA 'Screenpin/settings.json') -Raw | ConvertFrom-Json
  if ($settings.$field -ne $value) {throw "$field was not saved as $value"}
  $snapshot=Invoke-App 'capture'
  $memory=[System.IO.MemoryStream]::new([Convert]::FromBase64String($snapshot.png))
  $bitmap=[System.Drawing.Bitmap]::new($memory)
  try {return [int]$bitmap.GetPixel([int]($bitmap.Width*.55),[int]($bitmap.Height*.02)).A}
  finally {$bitmap.Dispose();$memory.Dispose()}
 }
 $low=Set-AppearanceSlider '背景透明度' 'glassTransparency' 19 'decrement' 69
 $high=Set-AppearanceSlider '背景透明度' 'glassTransparency' 24 'increment' 5
 if ($high -ge $low) {throw "Transparency adjustment did not change the backdrop ($low to $high)"}
 $lighter=Set-AppearanceSlider '磨砂浓度' 'glassDensity' 17 'decrement' 1
 $stronger=Set-AppearanceSlider '磨砂浓度' 'glassDensity' 38 'increment' 21
 if ($stronger -le $lighter+3) {throw "Matte strength did not visibly change the tint ($lighter to $stronger)"}
 $null=Set-AppearanceSlider '磨砂浓度' 'glassDensity' 18 'decrement' 20
 if ([Environment]::OSVersion.Version.Build -ge 22621) {
  function Assert-NativeMaterial($expected) {
   $process.Refresh()
   $material=0
   $hr=[ScreenpinTestFocus]::DwmGetWindowAttribute($process.MainWindowHandle,38,[ref]$material,4)
   if ($hr -ne 0 -or $material -ne $expected) {throw "Expected DWM material $expected, got $material (HRESULT $hr)"}
  }
  $null=Set-AppearanceSlider '背景透明度' 'glassTransparency' 29 'increment' 5
  Assert-NativeMaterial 3
  $at30=Set-AppearanceSlider '背景透明度' 'glassTransparency' 30 'increment' 1
  Assert-NativeMaterial 1
  $at65=Set-AppearanceSlider '背景透明度' 'glassTransparency' 65 'increment' 35
  $at100=Set-AppearanceSlider '背景透明度' 'glassTransparency' 100 'increment' 35
  Assert-NativeMaterial 1
  if ($at100 -gt 26 -or $at30-$at65 -notin 95..108 -or $at65-$at100 -notin 95..108) {
   throw "Tint did not fade through the upper range ($at30, $at65, $at100)"
  }
  $form=[System.Windows.Forms.Form]::new()
  $form.FormBorderStyle='None'
  $form.StartPosition='Manual'
  $form.ShowInTaskbar=$false
  $form.TopMost=$true
  $process.Refresh()
  $handle=$process.MainWindowHandle
  $rect=[ScreenpinTestFocus+NativeRect]::new()
  [ScreenpinTestFocus]::GetWindowRect($handle,[ref]$rect) | Out-Null
  $form.SetBounds($rect.Left,$rect.Top,$rect.Right-$rect.Left,$rect.Bottom-$rect.Top)
  $pixel=[System.Drawing.Bitmap]::new(1,1)
  $g=[System.Drawing.Graphics]::FromImage($pixel)
  try {
   $form.Show()
   [ScreenpinTestFocus]::SetWindowPos($handle,[IntPtr](-1),0,0,0,0,0x13) | Out-Null
   [ScreenpinTestFocus]::SetForegroundWindow($handle) | Out-Null
   function Measure-Backdrop {
    $colors=@(foreach ($color in @([System.Drawing.Color]::Red,[System.Drawing.Color]::Blue)) {
     $form.BackColor=$color
     $form.Refresh()
     Start-Sleep -Milliseconds 250
     $g.CopyFromScreen([int](($rect.Left+$rect.Right)/2),[int]($rect.Top+($rect.Bottom-$rect.Top)*.12),0,0,$pixel.Size)
     $pixel.GetPixel(0,0)
    })
    return [Math]::Min(($colors[0].R-$colors[1].R),($colors[1].B-$colors[0].B))
   }
   $visible100=Measure-Backdrop
   $null=Set-AppearanceSlider '背景透明度' 'glassTransparency' 65 'decrement' 35
   $visible65=Measure-Backdrop
   $null=Set-AppearanceSlider '背景透明度' 'glassTransparency' 30 'decrement' 35
   $visible30=Measure-Backdrop
   $null=Set-AppearanceSlider '背景透明度' 'glassTransparency' 29 'decrement' 1
   $visible29=Measure-Backdrop
   Write-Output "Desktop color transmission at 29/30/65/100: $visible29/$visible30/$visible65/$visible100"
   if ([Math]::Abs($visible30-$visible29) -gt 18 -or $visible100 -lt 205 -or $visible100-$visible65 -lt 60 -or $visible65-$visible30 -lt 60) {
    throw 'The desktop did not become steadily more visible at high transparency'
   }
  } finally {
   [ScreenpinTestFocus]::SetWindowPos($handle,[IntPtr](-2),0,0,0,0,0x13) | Out-Null
   $g.Dispose()
   $pixel.Dispose()
   $form.Dispose()
  }
  Assert-NativeMaterial 3
 }
 Click-Label '深色外观'
 Assert-Backdrop $true
 Capture 'settings-glass-dark-current.png'
 Write-Output 'PASS: native window appearance, transparency, and dark mode.'
} finally {
 $stream.Dispose()
 if (!$process.HasExited) {Stop-Process -Id $process.Id}
}
