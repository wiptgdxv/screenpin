param([string]$AppFile = 'artifacts/screenpin-check.exe', [switch]$ClipboardOnly)
$ErrorActionPreference = 'Stop'
$projectDir = Split-Path $PSScriptRoot -Parent
$artifactDir = Join-Path $projectDir 'artifacts'
New-Item -ItemType Directory -Force -Path $artifactDir | Out-Null
$env:APPDATA = Join-Path $env:TEMP ('screenpin-settings-test-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $env:APPDATA | Out-Null
$env:LOCALAPPDATA = $env:APPDATA
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

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class ScreenpinCaptureTest {
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern IntPtr FindWindow(string className,string title);
 [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr window,uint message,IntPtr wparam,IntPtr lparam);
 [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr window);
 [DllImport("user32.dll")] public static extern void keybd_event(byte key,byte scan,uint flags,UIntPtr extra);
 [DllImport("user32.dll")] public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr context);
 [DllImport("user32.dll")] public static extern int GetSystemMetrics(int index);
 [DllImport("user32.dll")] public static extern bool OpenClipboard(IntPtr owner);
 [DllImport("user32.dll")] public static extern bool CloseClipboard();
 [DllImport("user32.dll")] public static extern IntPtr GetClipboardData(uint format);
 [DllImport("kernel32.dll")] public static extern IntPtr GlobalLock(IntPtr memory);
 [DllImport("kernel32.dll")] public static extern bool GlobalUnlock(IntPtr memory);
 [DllImport("kernel32.dll")] public static extern UIntPtr GlobalSize(IntPtr memory);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern uint RegisterClipboardFormat(string name);
 [DllImport("user32.dll")] public static extern uint GetClipboardSequenceNumber();
 public static byte[] ReadPNG() {
  if (!OpenClipboard(IntPtr.Zero)) return null;
  try {
   IntPtr handle=GetClipboardData(RegisterClipboardFormat("PNG"));
   if (handle==IntPtr.Zero) return null;
   IntPtr pointer=GlobalLock(handle);
   if (pointer==IntPtr.Zero) return null;
   try {byte[] data=new byte[(int)GlobalSize(handle).ToUInt64()];Marshal.Copy(pointer,data,0,data.Length);return data;}
   finally {GlobalUnlock(handle);}
  } finally {CloseClipboard();}
 }
 [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr window,out Rect rect);
 public struct Rect { public int Left,Top,Right,Bottom; }
}
'@
[ScreenpinCaptureTest]::SetThreadDpiAwarenessContext([IntPtr](-4)) | Out-Null
function Send-Hotkey([byte]$key) {
 foreach ($code in @([byte]0x11,[byte]0x12,$key)) {[ScreenpinCaptureTest]::keybd_event($code,0,0,[UIntPtr]::Zero)}
 foreach ($code in @($key,[byte]0x12,[byte]0x11)) {[ScreenpinCaptureTest]::keybd_event($code,0,2,[UIntPtr]::Zero)}
}
function Wait-Overlay {
 $timer=[Diagnostics.Stopwatch]::StartNew()
 do {
  $handle=[ScreenpinCaptureTest]::FindWindow('ScreenpinCapture','Screenpin · 区域截图')
  [System.Windows.Forms.Application]::DoEvents()
  if ($handle -ne [IntPtr]::Zero -and [ScreenpinCaptureTest]::IsWindowVisible($handle)) {return $handle}
  if ($timer.Elapsed.TotalSeconds -gt 10) {throw ('Capture overlay did not appear: '+((Invoke-App 'tree').nodes.label -join ' / '))}
  [System.Windows.Forms.Application]::DoEvents()
  Start-Sleep -Milliseconds 50
 } while ($true)
}
function Fill-Key($label,$value) {
 $node=(Invoke-App 'tree').nodes | Where-Object {$_.label -eq $label} | Select-Object -First 1
 if (!$node) {throw "Missing field $label"}
 Invoke-App 'act' @{id=$node.id;action='setValue';text=$value} | Out-Null
 Settle
}
$fixture=$null
try {
 Capture 'control-panel-current.png'
 if (!$ClipboardOnly) {
 Click-Label '设置'
 Fill-Key '区域截图快捷键' 'Ctrl+Alt+7'
 Fill-Key '剪贴板图片贴图快捷键' 'Ctrl+Alt+8'
 Fill-Key '显示面板快捷键' 'Ctrl+Alt+9'
 Click-Label '应用快捷键'
 if ((Invoke-App 'tree').nodes.label -notcontains '快捷键已应用并保存') {throw 'Could not register test shortcuts'}
 Click-Label '工作台'
 $process.Refresh()
 $mainHandle=$process.MainWindowHandle
 if ($mainHandle -eq [IntPtr]::Zero) {throw 'No main window handle'}
 Click-Label '收起到托盘'
 if ([ScreenpinCaptureTest]::IsWindowVisible($mainHandle)) {throw 'Panel did not hide'}
 [System.Windows.Forms.Clipboard]::SetText('Screenpin cancellation sentinel')
 Send-Hotkey 0x37
 $overlay=Wait-Overlay
 [ScreenpinCaptureTest]::PostMessage($overlay,0x204,[IntPtr]::Zero,[IntPtr]::Zero) | Out-Null
 Start-Sleep -Milliseconds 400
 if ([System.Windows.Forms.Clipboard]::GetText() -ne 'Screenpin cancellation sentinel') {throw 'Cancellation modified clipboard'}
 if ([ScreenpinCaptureTest]::IsWindowVisible($mainHandle)) {throw 'Cancellation opened the hidden panel'}
 }
 $fixture=[System.Windows.Forms.Form]::new()
 $fixture.FormBorderStyle='None';$fixture.TopMost=$true
 $fixture.StartPosition='Manual';$fixture.Location=[System.Drawing.Point]::new(160,160)
 $fixture.Size=[System.Drawing.Size]::new(640,400)
 $fixture.BackColor=[System.Drawing.Color]::FromArgb(28,120,220)
 $fixture.Show();$fixture.Refresh();[System.Windows.Forms.Application]::DoEvents()
 $rect=[ScreenpinCaptureTest+Rect]::new()
 [ScreenpinCaptureTest]::GetWindowRect($fixture.Handle,[ref]$rect) | Out-Null
 Start-Sleep -Milliseconds 300
 $fixture.Refresh();[System.Windows.Forms.Application]::DoEvents()
 $reference=[System.Drawing.Bitmap]::new(160,100)
 $referenceGraphics=[System.Drawing.Graphics]::FromImage($reference)
 $referenceGraphics.CopyFromScreen($rect.Left+40,$rect.Top+40,0,0,$reference.Size)
 $referencePixel=$reference.GetPixel(80,50)
 $referenceGraphics.Dispose();$reference.Dispose()
 $sequence=[ScreenpinCaptureTest]::GetClipboardSequenceNumber()
 if ($ClipboardOnly) {
  $startAction=(Invoke-App 'tree').nodes | Where-Object {$_.label -eq '开始截图' -and $_.actions -contains 'press'} | Select-Object -First 1
  if (!$startAction) {throw 'Capture action missing'}
  Invoke-App 'act' @{id=$startAction.id;action='press'} | Out-Null
 } else { Send-Hotkey 0x37 }
 $overlay=Wait-Overlay
 $left=$rect.Left+40-[ScreenpinCaptureTest]::GetSystemMetrics(76)
 $top=$rect.Top+40-[ScreenpinCaptureTest]::GetSystemMetrics(77)
 $start=($left -band 65535) -bor (($top -band 65535) -shl 16)
 $end=(($left+160) -band 65535) -bor ((($top+100) -band 65535) -shl 16)
 [ScreenpinCaptureTest]::PostMessage($overlay,0x201,[IntPtr]1,[IntPtr]$start) | Out-Null
 [ScreenpinCaptureTest]::PostMessage($overlay,0x200,[IntPtr]1,[IntPtr]$end) | Out-Null
 [ScreenpinCaptureTest]::PostMessage($overlay,0x202,[IntPtr]::Zero,[IntPtr]$end) | Out-Null
 $timer=[Diagnostics.Stopwatch]::StartNew()
 do {
  $pngBytes=$null
  if ([ScreenpinCaptureTest]::GetClipboardSequenceNumber() -ne $sequence) { $pngBytes=[ScreenpinCaptureTest]::ReadPNG() }
  if ($pngBytes) {break}
  if ($timer.Elapsed.TotalSeconds -gt 10) {throw ('Screenshot did not reach clipboard. Formats: '+([System.Windows.Forms.Clipboard]::GetDataObject().GetFormats() -join ', ')+' / '+((Invoke-App 'tree').nodes.label -join ' / '))}
  [System.Windows.Forms.Application]::DoEvents()
  Start-Sleep -Milliseconds 50
 } while ($true)
 $pngStream=[System.IO.MemoryStream]::new($pngBytes)
 $result=[System.Drawing.Bitmap]::new($pngStream)
 try {
  if ($result.Width -ne 160 -or $result.Height -ne 100) {throw "Wrong crop size $($result.Size)"}
  $pixel=$result.GetPixel(80,50)
  if ($pixel.ToArgb() -ne $referencePixel.ToArgb()) {throw "Wrong captured pixel $pixel; desktop reference $referencePixel"}
  $result.Save((Join-Path $artifactDir 'capture-verification.png'),[System.Drawing.Imaging.ImageFormat]::Png)
 } finally {$result.Dispose();$pngStream.Dispose()}
 $pasteTarget=[System.Windows.Forms.RichTextBox]::new()
 try {
  $null=$pasteTarget.Handle
  $pasteTarget.Paste()
  if ($pasteTarget.Rtf -notmatch '\\(pict|object)') {throw 'Native rich text paste did not insert an image'}
 } finally {$pasteTarget.Dispose()}
 $files=[System.Windows.Forms.Clipboard]::GetFileDropList()
 if ($files.Count -ne 1 -or ![System.IO.File]::Exists($files[0])) {throw 'Screenshot did not publish a native file drop'}
 if ([Convert]::ToBase64String([System.IO.File]::ReadAllBytes($files[0])) -ne [Convert]::ToBase64String($pngBytes)) {throw 'Clipboard file bytes differ from the PNG bitmap data'}
 if ($ClipboardOnly) {
  $pasteDirectory=Join-Path $env:APPDATA 'PasteTarget'
  New-Item -ItemType Directory -Path $pasteDirectory | Out-Null
  $shell=New-Object -ComObject Shell.Application
  $shell.Explore($pasteDirectory)
  $timer.Restart()
  do {
   $explorer=@($shell.Windows()) | Where-Object { $_.LocationURL -and ([uri]$_.LocationURL).LocalPath -eq $pasteDirectory } | Select-Object -First 1
   if ($explorer) {break}
   if ($timer.Elapsed.TotalSeconds -gt 12) {throw 'Explorer paste target did not open'}
   Start-Sleep -Milliseconds 100
  } while ($true)
  try {
   [ScreenpinTestFocus]::ShowWindow([IntPtr]$explorer.HWND,9) | Out-Null
   [ScreenpinTestFocus]::SetForegroundWindow([IntPtr]$explorer.HWND) | Out-Null
   Start-Sleep -Milliseconds 400
   [System.Windows.Forms.SendKeys]::SendWait('^v')
   $pasted=Join-Path $pasteDirectory ([System.IO.Path]::GetFileName($files[0]))
   $timer.Restart()
   while (![System.IO.File]::Exists($pasted)) {
    if ($timer.Elapsed.TotalSeconds -gt 10) {throw 'Explorer Ctrl+V did not create the screenshot file'}
    Start-Sleep -Milliseconds 100
   }
   if ([Convert]::ToBase64String([System.IO.File]::ReadAllBytes($pasted)) -ne [Convert]::ToBase64String($pngBytes)) {throw 'Explorer pasted a different PNG'}
  } finally { $explorer.Quit() }
  Write-Output 'PASS: 160x100 exact screenshot pixels, native rich-text image paste, and Explorer Ctrl+V creates an identical PNG.'
  return
 }
 if ([ScreenpinCaptureTest]::IsWindowVisible($mainHandle)) {throw 'Capture unnecessarily showed the panel'}
 $fixture.Close();$fixture.Dispose();$fixture=$null
 Send-Hotkey 0x38
 $timer.Restart()
 do {
  $windows=@((Invoke-App 'windows').windows)
  if ($windows.title -like '贴图*') {break}
  if ($timer.Elapsed.TotalSeconds -gt 10) {throw 'Clipboard pin hotkey failed in background'}
  [System.Windows.Forms.Application]::DoEvents()
  Start-Sleep -Milliseconds 50
 } while ($true)
 Send-Hotkey 0x39
 Start-Sleep -Milliseconds 400
 if (![ScreenpinCaptureTest]::IsWindowVisible($mainHandle)) {throw 'Show-panel shortcut failed'}
 [ScreenpinCaptureTest]::PostMessage($mainHandle,0x10,[IntPtr]::Zero,[IntPtr]::Zero) | Out-Null
 Start-Sleep -Milliseconds 300
 if ($process.HasExited -or [ScreenpinCaptureTest]::IsWindowVisible($mainHandle)) {throw 'Closing the panel did not preserve background service'}
 $second=Start-Process -FilePath (Join-Path $projectDir $AppFile) -WindowStyle Hidden -PassThru
 if (!$second.WaitForExit(5000)) {throw 'Second instance did not hand off'}
 Start-Sleep -Milliseconds 300
 if (![ScreenpinCaptureTest]::IsWindowVisible($mainHandle)) {throw 'Second launch did not restore panel'}
 $windows=@((Invoke-App 'windows').windows)
 $script:windowId=($windows | Where-Object {$_.title -like '*控制面板*'} | Select-Object -First 1).id
 Capture 'control-panel-verified.png'
 $exitNode=(Invoke-App 'tree').nodes | Where-Object {$_.label -eq '退出程序' -and $_.actions -contains 'press'} | Select-Object -First 1
 if (!$exitNode) {throw 'Exit action missing'}
 try {Invoke-App 'act' @{id=$exitNode.id;action='press'} | Out-Null} catch {if (!$process.HasExited) {throw}}
 if (!$process.WaitForExit(5000)) {throw 'Explicit exit did not terminate app'}
 Write-Output 'PASS: background capture, cancel preserves clipboard, 160x100 exact pixels, background pin/show, close-to-tray, single instance and exit.'
} finally {
 if ($fixture) {$fixture.Close();$fixture.Dispose()}
 $stream.Dispose()
 if (!$process.HasExited) {Stop-Process -Id $process.Id}
}
