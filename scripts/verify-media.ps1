param([string]$AppFile = 'artifacts/screenpin-check.exe', [switch]$RecordingOnly, [switch]$ScrollToolsOnly, [switch]$SkipPointerCheck, [switch]$ScrollLeft, [switch]$OcrOnly, [switch]$ScrollBoth, [switch]$OcrLong, [switch]$OcrWechat, [switch]$OcrWindows)
$ErrorActionPreference = 'Stop'
$projectDir = Split-Path $PSScriptRoot -Parent
$artifactDir = Join-Path $projectDir 'artifacts'
New-Item -ItemType Directory -Force -Path $artifactDir | Out-Null
$env:APPDATA = Join-Path $env:TEMP ('screenpin-settings-test-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $env:APPDATA | Out-Null
$env:MYGO_LOCALE = 'zh-Hans'
$env:SCREENPIN_VIDEO_DIR = Join-Path $artifactDir 'media-validation'
$exportDirectory = $env:SCREENPIN_VIDEO_DIR
if ($RecordingOnly) {
 $exportDirectory = Join-Path $artifactDir ('custom-recordings-' + [guid]::NewGuid().ToString('N'))
 $settingsDirectory = Join-Path $env:APPDATA 'Screenpin'
 New-Item -ItemType Directory -Force -Path $settingsDirectory | Out-Null
 @{videoSaveDirectory=$exportDirectory} | ConvertTo-Json | Set-Content (Join-Path $settingsDirectory 'settings.json') -Encoding utf8NoBOM
}
New-Item -ItemType Directory -Force -Path $env:SCREENPIN_VIDEO_DIR | Out-Null
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
 [DllImport("user32.dll")] public static extern bool SetCursorPos(int x,int y);
 [DllImport("user32.dll")] public static extern void mouse_event(uint flags,int x,int y,int data,UIntPtr extra);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern IntPtr FindWindow(string className,string title);
 [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr window,uint message,IntPtr wparam,IntPtr lparam);
 [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr window);
 [DllImport("user32.dll")] public static extern bool GetWindowDisplayAffinity(IntPtr window,out uint affinity);
 [DllImport("user32.dll")] public static extern void keybd_event(byte key,byte scan,uint flags,UIntPtr extra);
 [DllImport("user32.dll")] public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr context);
 [DllImport("user32.dll")] public static extern int GetSystemMetrics(int index);
 [DllImport("user32.dll")] public static extern uint GetClipboardSequenceNumber();
 [DllImport("user32.dll")] public static extern bool OpenClipboard(IntPtr owner);
 [DllImport("user32.dll")] public static extern bool CloseClipboard();
 [DllImport("user32.dll")] public static extern IntPtr GetClipboardData(uint format);
 [DllImport("kernel32.dll")] public static extern IntPtr GlobalLock(IntPtr memory);
 [DllImport("kernel32.dll")] public static extern bool GlobalUnlock(IntPtr memory);
 [DllImport("kernel32.dll")] public static extern UIntPtr GlobalSize(IntPtr memory);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern uint RegisterClipboardFormat(string name);
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
 [DllImport("user32.dll")] public static extern bool IsZoomed(IntPtr window);
 [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr window,IntPtr after,int x,int y,int width,int height,uint flags);
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

Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
using System.Threading;
public static class ScreenpinPatternSource {
 [UnmanagedFunctionPointer(CallingConvention.Winapi)] delegate IntPtr WindowProc(IntPtr h,uint m,IntPtr w,IntPtr l);
 static WindowProc callback=OnMessage;
 static Thread thread;
 static ManualResetEventSlim ready=new ManualResetEventSlim();
 static int offset,horizontal;
 public static IntPtr Handle;
 [StructLayout(LayoutKind.Sequential,CharSet=CharSet.Unicode)] struct WindowClass { public uint Style;public IntPtr Proc;public int Extra,WindowExtra;public IntPtr Instance,Icon,Cursor,Brush;public string Menu;public string Name; }
 [StructLayout(LayoutKind.Sequential)] struct Point {public int X,Y;}
 [StructLayout(LayoutKind.Sequential)] struct Message {public IntPtr Window;public uint Type;public IntPtr W,L;public uint Time;public Point Point;public uint Private;}
 [StructLayout(LayoutKind.Sequential)] struct Rect {public int Left,Top,Right,Bottom;}
 [StructLayout(LayoutKind.Sequential)] struct Paint {public IntPtr DC;public int Erase;public Rect Rect;public int Restore,Update;[MarshalAs(UnmanagedType.ByValArray,SizeConst=32)]public byte[] Reserved;}
 [StructLayout(LayoutKind.Sequential)] struct Info {public uint Size;public int Width,Height;public ushort Planes,Bits;public uint Compression,ImageSize;public int XPels,YPels;public uint Used,Important;}
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern ushort RegisterClass(ref WindowClass c);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern IntPtr CreateWindowEx(uint ex,string name,string title,uint style,int x,int y,int w,int h,IntPtr parent,IntPtr menu,IntPtr instance,IntPtr data);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern IntPtr DefWindowProc(IntPtr h,uint m,IntPtr w,IntPtr l);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern int GetMessage(out Message message,IntPtr window,uint min,uint max);
 [DllImport("user32.dll")] static extern bool TranslateMessage(ref Message message);
 [DllImport("user32.dll")] static extern IntPtr DispatchMessage(ref Message message);
 [DllImport("user32.dll")] static extern bool ShowWindow(IntPtr window,int command);
 [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr window);
 [DllImport("user32.dll")] static extern bool DestroyWindow(IntPtr window);
 [DllImport("user32.dll")] static extern void PostQuitMessage(int code);
 [DllImport("user32.dll")] static extern bool InvalidateRect(IntPtr window,IntPtr rect,bool erase);
 [DllImport("user32.dll")] static extern bool UpdateWindow(IntPtr window);
 [DllImport("user32.dll")] static extern IntPtr BeginPaint(IntPtr window,out Paint paint);
 [DllImport("user32.dll")] static extern bool EndPaint(IntPtr window,ref Paint paint);
 [DllImport("user32.dll")] static extern IntPtr SendMessage(IntPtr window,uint message,IntPtr w,IntPtr l);
 [DllImport("user32.dll")] static extern bool PostMessage(IntPtr window,uint message,IntPtr w,IntPtr l);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern IntPtr FindWindow(string className,string title);
 [DllImport("user32.dll")] static extern IntPtr SetThreadDpiAwarenessContext(IntPtr context);
 [DllImport("gdi32.dll")] static extern int SetDIBitsToDevice(IntPtr dc,int x,int y,uint w,uint h,int sx,int sy,uint start,uint lines,byte[] bits,ref Info info,uint colors);
 [DllImport("kernel32.dll",CharSet=CharSet.Unicode)] static extern IntPtr GetModuleHandle(string name);
 public static IntPtr FindTitle(string title){return FindWindow(null,title);}
 public static void Start(){
  ready.Reset();offset=0;horizontal=0;
  thread=new Thread(()=>{
   SetThreadDpiAwarenessContext(new IntPtr(-4));
   IntPtr instance=GetModuleHandle(null);
   var cls=new WindowClass{Proc=Marshal.GetFunctionPointerForDelegate(callback),Instance=instance,Name="ScreenpinPatternSource"};
   RegisterClass(ref cls);
   Handle=CreateWindowEx(0x88,"ScreenpinPatternSource","Screenpin scrolling test source",0x80000000,400,200,360,380,IntPtr.Zero,IntPtr.Zero,instance,IntPtr.Zero);
   ShowWindow(Handle,5);SetForegroundWindow(Handle);UpdateWindow(Handle);ready.Set();
   Message message;while(GetMessage(out message,IntPtr.Zero,0,0)>0){TranslateMessage(ref message);DispatchMessage(ref message);}
  });thread.IsBackground=true;thread.SetApartmentState(ApartmentState.STA);thread.Start();
  if(!ready.Wait(5000)||Handle==IntPtr.Zero)throw new Exception("Pattern window failed");
 }
 public static void Front(){SetForegroundWindow(Handle);}
 public static void HorizontalWheel(int delta){PostMessage(Handle,0x20e,new IntPtr(delta<<16),IntPtr.Zero);}
 public static void SetHorizontal(int value){SendMessage(Handle,0x8009,new IntPtr(value),IntPtr.Zero);}
 public static void SetOffset(int value){SendMessage(Handle,0x8008,new IntPtr(value),IntPtr.Zero);}
 public static void Wheel(){PostMessage(Handle,0x20a,new IntPtr(-120<<16),IntPtr.Zero);}
 public static void Stop(){if(Handle!=IntPtr.Zero){PostMessage(Handle,0x10,IntPtr.Zero,IntPtr.Zero);thread.Join(3000);Handle=IntPtr.Zero;}}
 static IntPtr OnMessage(IntPtr h,uint m,IntPtr w,IntPtr l){
  if(m==0x14)return new IntPtr(1);
  if(m==0x8009||m==0x20e){horizontal=m==0x8009?w.ToInt32():Math.Clamp(horizontal+((short)(w.ToInt64()>>16)>0?45:-45),0,90);InvalidateRect(h,IntPtr.Zero,false);UpdateWindow(h);return IntPtr.Zero;}
  if(m==0x8008||m==0x20a){offset=m==0x8008?w.ToInt32():Math.Min(1000,offset+45);InvalidateRect(h,IntPtr.Zero,false);UpdateWindow(h);return IntPtr.Zero;}
  if(m==0xf){
   Paint paint;IntPtr dc=BeginPaint(h,out paint);byte[] pixels=new byte[360*380*4];
   for(int y=0;y<380;y++)for(int x=0;x<360;x++){int row=y+offset,column=x+horizontal,i=(y*360+x)*4;pixels[i]=(byte)((column*row*7+row*23)%256);pixels[i+1]=(byte)((column*43+row*row*3)%256);pixels[i+2]=(byte)((column*13+row*37)%256);pixels[i+3]=255;}
   var info=new Info{Size=40,Width=360,Height=-380,Planes=1,Bits=32};SetDIBitsToDevice(dc,0,0,360,380,0,0,0,380,pixels,ref info,0);EndPaint(h,ref paint);return IntPtr.Zero;
  }
  if(m==0x10){DestroyWindow(h);return IntPtr.Zero;}if(m==2){PostQuitMessage(0);return IntPtr.Zero;}
  return DefWindowProc(h,m,w,l);
 }
}
'@

function Select-Pattern($width=160,$height=200,[switch]$fullDesktop) {
 $overlay=Wait-Overlay
 $left=410-[ScreenpinCaptureTest]::GetSystemMetrics(76)
 $top=210-[ScreenpinCaptureTest]::GetSystemMetrics(77)
 if ($fullDesktop) {
  $left=0;$top=0
  $width=[ScreenpinCaptureTest]::GetSystemMetrics(78)
  $height=[ScreenpinCaptureTest]::GetSystemMetrics(79)
 }
 $start=($left -band 65535) -bor (($top -band 65535) -shl 16)
 $end=(($left+$width) -band 65535) -bor ((($top+$height) -band 65535) -shl 16)
 [ScreenpinCaptureTest]::PostMessage($overlay,0x201,[IntPtr]1,[IntPtr]$start) | Out-Null
 [ScreenpinCaptureTest]::PostMessage($overlay,0x200,[IntPtr]1,[IntPtr]$end) | Out-Null
 [ScreenpinCaptureTest]::PostMessage($overlay,0x202,[IntPtr]::Zero,[IntPtr]$end) | Out-Null
}
function Read-NewPNG($sequence) {
 $timer=[Diagnostics.Stopwatch]::StartNew()
 do {
  if ([ScreenpinCaptureTest]::GetClipboardSequenceNumber() -ne $sequence) {
   $bytes=[ScreenpinCaptureTest]::ReadPNG()
   if ($bytes) {return ,$bytes}
  }
  if ($timer.Elapsed.TotalSeconds -gt 15) {throw 'No new PNG reached clipboard'}
  Start-Sleep -Milliseconds 50
 } while ($true)
}
function Assert-Pattern($bytes,$height) {
 $memory=[System.IO.MemoryStream]::new($bytes);$image=[System.Drawing.Bitmap]::new($memory)
 try {
  if ($image.Width -ne 160 -or $image.Height -ne $height) {throw "Wrong pattern image size $($image.Size), expected 160x$height"}
  for ($y=0;$y -lt $height;$y++) {for ($x=0;$x -lt 160;$x++) {
   $row=$y+10;$column=$x+10;$pixel=$image.GetPixel($x,$y)
   if ($pixel.R -ne (($column*13+$row*37)%256) -or $pixel.G -ne (($column*43+$row*$row*3)%256) -or $pixel.B -ne (($column*$row*7+$row*23)%256)) {throw "Pattern pixel mismatch at $x,$y : $pixel"}
  }}
 } finally {$image.Dispose();$memory.Dispose()}
}
function Main-Window {
 $script:windowId=(@((Invoke-App 'windows').windows) | Where-Object {$_.title -like '*控制面板*'} | Select-Object -First 1).id
}
function Show-TestPanel {
 if (!$RecordingOnly) { Send-Hotkey 0x39; return }
 $main=[ScreenpinCaptureTest]::FindWindow([NullString]::Value,'映刻 Screenpin · 控制面板')
 [ScreenpinTestFocus]::ShowWindow($main,9) | Out-Null
 [ScreenpinTestFocus]::SetForegroundWindow($main) | Out-Null
}
function Start-RecordingTest {
 if (!$RecordingOnly) { Send-Hotkey 0x36; return }
 Show-TestPanel;Main-Window;Settle
 $record=(Invoke-App 'tree').nodes | Where-Object {$_.label -eq '开始录屏' -and $_.actions -contains 'press'} | Select-Object -First 1
 if (!$record) {throw 'Recording action missing'}
 Invoke-App 'act' @{id=$record.id;action='press'} | Out-Null
}
function Wait-RecordPreview {
 $timer=[Diagnostics.Stopwatch]::StartNew()
 do {
  $preview=@((Invoke-App 'windows').windows) | Where-Object {$_.title -eq '录屏预览'} | Select-Object -First 1
  if ($preview) {
   $script:windowId=$preview.id
   try { if ((Invoke-App 'eval' @{script='document.querySelector("video")?.readyState >= 1'}).value) { return } } catch {}
  }
  if ($timer.Elapsed.TotalSeconds -gt 20) {throw 'Playable recording preview did not appear'}
  Start-Sleep -Milliseconds 150
 } while ($true)
}
function Wait-PreviewSaved {
 $timer=[Diagnostics.Stopwatch]::StartNew()
 do {
  $saved=(Invoke-App 'eval' @{script='document.querySelector("#status").textContent.startsWith("已保存")'}).value
  if (!$saved) { Start-Sleep -Milliseconds 100 }
  if ($timer.Elapsed.TotalSeconds -gt 15) {throw 'Save from preview did not finish'}
 } until ($saved)
}
function Wait-Pin($present) {
 $timer=[Diagnostics.Stopwatch]::StartNew()
 do {
  $pin=@((Invoke-App 'windows').windows) | Where-Object {$_.title -like '贴图*'} | Select-Object -First 1
  if (($null -ne $pin) -eq $present) {return $pin}
  if ($timer.Elapsed.TotalSeconds -gt 5) {throw "Pin presence did not become $present"}
  Start-Sleep -Milliseconds 50
 } while ($true)
}


try {
 Capture 'control-panel-media.png'
 Click-Label '设置'
 Click-Label '快捷键'
 Fill-Key '区域截图快捷键' 'Ctrl+Alt+7'
 Fill-Key '剪贴板图片贴图快捷键' 'Ctrl+Alt+8'
 Fill-Key '显示面板快捷键' 'Ctrl+Alt+9'
 Fill-Key '滚动截图 / 完成快捷键' 'Ctrl+Alt+5'
 Fill-Key '录屏 / 停止快捷键' 'Ctrl+Alt+6'
 Click-Label '应用快捷键'
 if ((Invoke-App 'tree').nodes.label -notcontains '快捷键已应用并保存') {throw 'Test shortcuts did not apply'}
 Click-Label '工作台'
 if ($OcrOnly) {
  $bitmap=if ($OcrLong) {[Drawing.Bitmap]::new(420,6500)} else {[Drawing.Bitmap]::new(1024,220)}
  $graphics=[Drawing.Graphics]::FromImage($bitmap)
  $fontSize=if ($OcrLong) {13} else {54}
  $font=[Drawing.Font]::new('Microsoft YaHei UI',$fontSize,[Drawing.FontStyle]::Bold)
  try {
   $graphics.TextRenderingHint=[Drawing.Text.TextRenderingHint]::AntiAliasGridFit
   $graphics.Clear([Drawing.Color]::White)
   $graphics.DrawString('中文测试 截图工具 123',$font,[Drawing.Brushes]::Black,34,54)
   if ($OcrLong) {
    $graphics.DrawString('Section Alpha 987',$font,[Drawing.Brushes]::Black,34,1588)
    $graphics.DrawString('Section Omega 654',$font,[Drawing.Brushes]::Black,34,6320)
   }
   [Windows.Forms.Clipboard]::SetImage($bitmap)
  } finally {$graphics.Dispose();$font.Dispose();$bitmap.Dispose()}
  Invoke-App 'input' @{events=@(@{type='key';key='Ctrl+V'})} | Out-Null
  Settle
  $ocr=(Invoke-App 'tree').nodes | Where-Object {$_.label -eq '提取文字' -and $_.actions -contains 'press'} | Select-Object -First 1
  if (!$ocr) {throw 'OCR action missing from image view'}
  Invoke-App 'act' @{id=$ocr.id;action='press'} | Out-Null
  $timer=[Diagnostics.Stopwatch]::StartNew()
  do {
   $window=@((Invoke-App 'windows').windows) | Where-Object {$_.title -eq '提取文字'} | Select-Object -First 1
   if ($window) {
    $script:windowId=$window.id
    Settle
    $ocrNodes=(Invoke-App 'tree').nodes
    $result=$ocrNodes | Where-Object {$_.label -eq '识别结果'} | Select-Object -First 1
   }
   if ($timer.Elapsed.TotalSeconds -gt 15) {throw "OCR result missing: $($ocrNodes.label -join ' / ')"}
  } until ($result)
  if ($OcrWechat -and !($ocrNodes.label | Where-Object {$_ -like '微信 OCR ·*'})) {throw 'Result did not use WeChat OCR'}
  if ($OcrWindows -and !($ocrNodes.label | Where-Object {$_ -like 'Windows OCR ·*'})) {throw 'Result did not fall back to Windows OCR'}
  foreach ($label in @('识别引擎','识别语言')) {if ($ocrNodes.label -notcontains $label) {throw "Missing OCR selector $label"}}
  Capture 'ocr-result-verified.png'
  $copy=$ocrNodes | Where-Object {$_.label -eq '复制全部' -and $_.actions -contains 'press'} | Select-Object -First 1
  if (!$copy) {throw 'OCR copy action missing'}
  Invoke-App 'act' @{id=$copy.id;action='press'} | Out-Null
  Settle
  $copied=[Windows.Forms.Clipboard]::GetText()
  if ($copied -notmatch '中文测试.*截图工具.*123') {throw "Wrong OCR text copied: $copied"}
  if ($OcrLong -and ($copied -notmatch '(?s)123.*Section Alpha 987.*Section Omega 654' -or [regex]::Matches($copied,'Section Alpha 987').Count -ne 1)) {
   throw "Long OCR missed, reordered or duplicated lines: $copied"
  }
  $resultHandle=[ScreenpinCaptureTest]::FindWindow([NullString]::Value,'提取文字')
  [ScreenpinCaptureTest]::PostMessage($resultHandle,0x10,[IntPtr]::Zero,[IntPtr]::Zero) | Out-Null
  Main-Window
  Click-Label '工作台'
  Click-Label '提取文字'
  $timer.Restart()
  do {
   $window=@((Invoke-App 'windows').windows) | Where-Object {$_.title -eq '提取文字'} | Select-Object -First 1
   if ($window) {
    $script:windowId=$window.id
    Settle
    $result=(Invoke-App 'tree').nodes | Where-Object {$_.label -eq '识别结果'} | Select-Object -First 1
   } else { $result=$null; Start-Sleep -Milliseconds 100 }
   if ($timer.Elapsed.TotalSeconds -gt 15) {throw 'Workbench OCR did not show a result'}
  } until ($result)
  Write-Output 'PASS: built-in Chinese OCR from the image view and workbench, readable result and clipboard copy.'
  return
 }
 if (!$RecordingOnly -and !$ScrollToolsOnly) { Click-Label '收起到托盘' }
 [ScreenpinPatternSource]::Start()
 Start-Sleep -Milliseconds 300
 $reference=[System.Drawing.Bitmap]::new(1,1);$graphics=[System.Drawing.Graphics]::FromImage($reference)
 $graphics.CopyFromScreen(420,220,0,0,$reference.Size);$pixel=$reference.GetPixel(0,0);$graphics.Dispose();$reference.Dispose()
 if ($pixel.R -ne ((20*13+20*37)%256)) {throw "Desktop pattern source not visible: $pixel"}
 if ($ScrollToolsOnly) {
  Main-Window;Settle
  $start=(Invoke-App 'tree').nodes | Where-Object {$_.label -eq '开始滚动' -and $_.actions -contains 'press'} | Select-Object -First 1
  if (!$start) {throw 'Scroll launcher missing'}
  $sequence=[ScreenpinCaptureTest]::GetClipboardSequenceNumber()
  if ($ScrollBoth) {[ScreenpinPatternSource]::SetOffset(90)}
  if ($ScrollLeft) {[ScreenpinPatternSource]::SetHorizontal(90)}
  Invoke-App 'act' @{id=$start.id;action='press'} | Out-Null
  Select-Pattern 160 200
  function Wait-ScrollLabel($match) {
   $timer=[Diagnostics.Stopwatch]::StartNew()
   do {
    $task=@((Invoke-App 'windows').windows) | Where-Object {$_.title -eq '滚动截图'} | Select-Object -First 1
    if ($task) {
     $script:windowId=$task.id
     Settle
     $nodes=(Invoke-App 'tree').nodes
     $node=$nodes | Where-Object {$_.label -match $match} | Select-Object -First 1
     if ($node) {return $node}
    }
    if ($timer.Elapsed.TotalSeconds -gt 10) {throw "Scroll status $match missing: $($nodes.label -join ' / ')"}
   } while ($true)
  }
  function Press-Scroll($label) {
   $node=Wait-ScrollLabel ('^'+$label+'$')
   Invoke-App 'act' @{id=$node.id;action='press'} | Out-Null
  }
  $null=Wait-ScrollLabel '即可识别方向'
  $toolbarLabels=(Invoke-App 'tree').nodes.label
  if ($toolbarLabels | Where-Object {$_ -in @('向上','向下','向左','向右','自动','停自动')}) {throw 'Scroll toolbar has manual direction or auto-scroll controls'}
  Capture 'scroll-toolbar-verified.png'
  $scrollControl=[ScreenpinCaptureTest]::FindWindow([NullString]::Value,'滚动截图')
  $scrollRect=[ScreenpinCaptureTest+Rect]::new()
  if (![ScreenpinCaptureTest]::GetWindowRect($scrollControl,[ref]$scrollRect) -or $scrollRect.Left -ne 410 -or $scrollRect.Top -ne 416) {
   throw "Scroll controls did not dock below the selection (left=$($scrollRect.Left), top=$($scrollRect.Top))"
  }
  if ($ScrollBoth) {
   foreach ($step in @(@(45,245),@(0,290),@(45,290),@(90,290),@(135,335),@(180,380))) {
    [ScreenpinPatternSource]::SetOffset($step[0])
    Start-Sleep -Milliseconds 200
    $null=Wait-ScrollLabel ('160 × '+$step[1]+' · 上下拼接')
   }
   Press-Scroll '完成'
   $bytes=Read-NewPNG $sequence
   Assert-Pattern $bytes 380
   Write-Output 'PASS: one capture extended upward and downward from the middle with 160x380 exact pixels.'
   return
  }
  $direction=if ($ScrollLeft) {'向左'} else {'向右'}
  if ($ScrollLeft) {[ScreenpinPatternSource]::HorizontalWheel(-120)} else {[ScreenpinPatternSource]::HorizontalWheel(120)}
  $null=Wait-ScrollLabel '205 × 200 · 左右拼接'
  if ($ScrollLeft) {[ScreenpinPatternSource]::HorizontalWheel(-120)} else {[ScreenpinPatternSource]::HorizontalWheel(120)}
  $null=Wait-ScrollLabel '250 × 200'
  Press-Scroll '撤回'
  $null=Wait-ScrollLabel '已撤回'
  Press-Scroll '完成'
  $bytes=Read-NewPNG $sequence
  [IO.File]::WriteAllBytes((Join-Path $artifactDir 'scroll-horizontal-verification.png'),$bytes)
  $memory=[IO.MemoryStream]::new($bytes);$img=[Drawing.Bitmap]::new($memory)
  try {
   if ($img.Width -ne 205 -or $img.Height -ne 200) {throw "Wrong horizontal size after undo: $($img.Size)"}
   for ($y=0;$y -lt 200;$y++) {for($x=0;$x -lt 205;$x++) {
    $row=$y+10;$col=$x+10;if ($ScrollLeft) {$col+=45};$pixel=$img.GetPixel($x,$y)
    if ($pixel.R -ne (($col*13+$row*37)%256) -or $pixel.G -ne (($col*43+$row*$row*3)%256) -or $pixel.B -ne (($col*$row*7+$row*23)%256)) {throw "Wrong horizontal pixel at $x,$y : $pixel"}
   }}
  } finally {$img.Dispose();$memory.Dispose()}
  Write-Output "PASS: detected $direction, compact toolbar, undo, and all pixels in the retained 205x200 PNG."
  return
 }
 if (!$RecordingOnly) {
 $sequence=[ScreenpinCaptureTest]::GetClipboardSequenceNumber()
 Send-Hotkey 0x37;Select-Pattern
 $bytes=Read-NewPNG $sequence;Assert-Pattern $bytes 200
 $pin=Wait-Pin $true
 [ScreenpinPatternSource]::Front();$null=Wait-Pin $false
 Send-Hotkey 0x38
 $pin=Wait-Pin $true
 [ScreenpinPatternSource]::Front();Start-Sleep -Milliseconds 250
 $null=Wait-Pin $true
 $script:windowId=$pin.id
 [ScreenpinTestFocus]::SetForegroundWindow([ScreenpinPatternSource]::FindTitle($pin.title)) | Out-Null
 Invoke-App 'input' @{events=@(@{type='key';key='Escape'})} | Out-Null
 $null=Wait-Pin $false
 $pinFile=Join-Path $artifactDir '桌面复制测试.png'
 [System.IO.File]::WriteAllBytes($pinFile,$bytes)
 $fileDrop=[System.Collections.Specialized.StringCollection]::new();$null=$fileDrop.Add($pinFile)
 [System.Windows.Forms.Clipboard]::SetFileDropList($fileDrop)
 Send-Hotkey 0x38
 $pin=Wait-Pin $true
 if ($pin.title -notlike '*桌面复制测试.png') {throw 'Copied image file did not become a pin'}
 $script:windowId=$pin.id
 [ScreenpinTestFocus]::SetForegroundWindow([ScreenpinPatternSource]::FindTitle($pin.title)) | Out-Null
 Invoke-App 'input' @{events=@(@{type='key';key='Escape'})} | Out-Null
 $null=Wait-Pin $false
 Send-Hotkey 0x39;Main-Window
 Click-Label '收起到托盘'
 [ScreenpinPatternSource]::Front()
 $sequence=[ScreenpinCaptureTest]::GetClipboardSequenceNumber()
 Send-Hotkey 0x37;Select-Pattern
 $bytes=Read-NewPNG $sequence;$null=Wait-Pin $true
 [ScreenpinPatternSource]::Front();$null=Wait-Pin $false
 $sequence=[ScreenpinCaptureTest]::GetClipboardSequenceNumber()
 Send-Hotkey 0x35;Select-Pattern
 Start-Sleep -Milliseconds 1500
 if ([ScreenpinCaptureTest]::FindWindow('ScreenpinRegionOutline','Screenpin 选区边框') -eq [IntPtr]::Zero) {throw 'Scroll outline missing'}
 for ($index=1;$index -le 3;$index++) {
  [ScreenpinPatternSource]::Wheel()
  $expected=200+45*$index;$timer=[Diagnostics.Stopwatch]::StartNew()
  do {
   $task=@((Invoke-App 'windows').windows) | Where-Object {$_.title -eq '滚动截图'} | Select-Object -First 1
   if ($task) {$script:windowId=$task.id;Settle;$labels=(Invoke-App 'tree').nodes.label}
   if (($labels -join ' ') -match "$expected") {break}
   if ($timer.Elapsed.TotalSeconds -gt 8) {throw "Scroll frame not accepted: $($labels -join ' / ')"}
   Start-Sleep -Milliseconds 100
  } while ($true)
 }
 [ScreenpinPatternSource]::SetOffset(180)
 Send-Hotkey 0x35
 $bytes=Read-NewPNG $sequence;Assert-Pattern $bytes 380
 if ([ScreenpinCaptureTest]::FindWindow('ScreenpinRegionOutline','Screenpin 选区边框') -ne [IntPtr]::Zero) {throw 'Scroll outline remained after completion'}
 [System.IO.File]::WriteAllBytes((Join-Path $artifactDir 'scroll-verification.png'),$bytes)
 }
 [ScreenpinPatternSource]::Front();Start-Sleep -Milliseconds 300
 $before=@(Get-ChildItem -LiteralPath $exportDirectory -ErrorAction SilentlyContinue -Filter '*.mp4' | Where-Object {$_.Name -notlike '*.partial.mp4'}).FullName
 [ScreenpinPatternSource]::SetOffset(0)
 Start-RecordingTest;Select-Pattern 317 239
 Start-Sleep -Milliseconds 1500
 if ([ScreenpinCaptureTest]::FindWindow('ScreenpinRegionOutline','Screenpin 选区边框') -eq [IntPtr]::Zero) {throw 'Recording outline missing'}
 # A short quiet tone gives the loopback test a known signal.
 $toneStream=[IO.MemoryStream]::new()
 $toneWriter=[IO.BinaryWriter]::new($toneStream)
 $samples=19200
 $toneWriter.Write([Text.Encoding]::ASCII.GetBytes('RIFF'));$toneWriter.Write([int](36+$samples*2))
 $toneWriter.Write([Text.Encoding]::ASCII.GetBytes('WAVEfmt '));$toneWriter.Write([int]16)
 $toneWriter.Write([int16]1);$toneWriter.Write([int16]1);$toneWriter.Write([int]48000);$toneWriter.Write([int]96000)
 $toneWriter.Write([int16]2);$toneWriter.Write([int16]16);$toneWriter.Write([Text.Encoding]::ASCII.GetBytes('data'));$toneWriter.Write([int]($samples*2))
 for ($i=0;$i -lt $samples;$i++) { $toneWriter.Write([int16](3200*[Math]::Sin(2*[Math]::PI*523*$i/48000))) }
 $toneStream.Position=0
 $player=[System.Media.SoundPlayer]::new($toneStream)
 try { $player.PlaySync() } finally { $player.Dispose();$toneWriter.Dispose();$toneStream.Dispose() }
 for ($index=1;$index -le 7;$index++) {[ScreenpinPatternSource]::SetOffset($index*25);Start-Sleep -Milliseconds 250}
 $task=@((Invoke-App 'windows').windows) | Where-Object {$_.title -eq '区域录屏'} | Select-Object -First 1
 $control=[ScreenpinCaptureTest]::FindWindow([NullString]::Value,'区域录屏')
 $rect=[ScreenpinCaptureTest+Rect]::new()
 if (!$task -or ![ScreenpinCaptureTest]::GetWindowRect($control,[ref]$rect) -or $rect.Top -ne 455) {throw "Recording controls did not dock below the selected area (window=$control, top=$($rect.Top), task=$($task.id))"}
 $script:windowId=$task.id
 Capture 'recording-toolbar-verified.png'
 $pause=(Invoke-App 'tree').nodes | Where-Object {$_.label -eq '暂停录制' -and $_.actions -contains 'press'} | Select-Object -First 1
 if (!$pause) {throw 'Pause action missing'}
 Invoke-App 'act' @{id=$pause.id;action='press'} | Out-Null
 $pauseTimer=[Diagnostics.Stopwatch]::StartNew()
 do {
  Settle
  $resume=(Invoke-App 'tree').nodes | Where-Object {$_.label -eq '继续录制' -and $_.actions -contains 'press'} | Select-Object -First 1
  if ($pauseTimer.Elapsed.TotalSeconds -gt 12) {throw 'Recorder did not pause'}
 } until ($resume)
 if (Get-ChildItem -LiteralPath (Join-Path $env:SCREENPIN_VIDEO_DIR '.drafts') -Filter '*.partial.mp4') {throw 'Encoder continued writing while paused'}
 Start-Sleep -Milliseconds 800
 Invoke-App 'act' @{id=$resume.id;action='press'} | Out-Null
 $resumeTimer=[Diagnostics.Stopwatch]::StartNew()
 do {
  [ScreenpinPatternSource]::SetOffset(240 + ($resumeTimer.ElapsedMilliseconds -shr 3))
  Settle
  $recording=(Invoke-App 'tree').nodes | Where-Object {$_.label -eq '录屏中'} | Select-Object -First 1
  if ($resumeTimer.Elapsed.TotalSeconds -gt 12) {throw 'Recorder did not resume'}
 } until ($recording)
 $stop=(Invoke-App 'tree').nodes | Where-Object {$_.label -eq '结束录制' -and $_.actions -contains 'press'} | Select-Object -First 1
 if (!$stop) {throw 'Docked stop action missing'}
 Invoke-App 'act' @{id=$stop.id;action='press'} | Out-Null
 Wait-RecordPreview
 $null=Invoke-App 'eval' @{script='document.querySelector("a[href=close]").click()'}
 $closeTimer=[Diagnostics.Stopwatch]::StartNew()
 do {
  $opened=(Invoke-App 'eval' @{script='document.querySelector("#close-confirmation").open'}).value
  if (!$opened) {Start-Sleep -Milliseconds 80}
  if ($closeTimer.Elapsed.TotalSeconds -gt 5) {throw 'Unsaved close confirmation did not open in preview'}
 } until ($opened)
 Capture 'recording-close-confirmation.png'
 $null=Invoke-App 'eval' @{script='document.querySelector("#keep-preview").click()'}
 Start-Sleep -Milliseconds 250
 if ((Invoke-App 'eval' @{script='document.querySelector("#close-confirmation").open'}).value) {throw 'Continue did not dismiss close confirmation'}
 $null=Invoke-App 'eval' @{script='document.querySelector("#sound").click()'}
 if (!(Invoke-App 'eval' @{script='document.querySelector("video").muted'}).value) {throw 'Preview mute button did not mute playback'}
 $null=Invoke-App 'eval' @{script='document.querySelector("#sound").click()'}
 if ((Invoke-App 'eval' @{script='document.querySelector("video").muted'}).value) {throw 'Preview mute button did not restore playback sound'}
 if (Get-ChildItem -LiteralPath $exportDirectory -ErrorAction SilentlyContinue -Filter '*.mp4' | Where-Object {$_.Name -notlike '*.partial.mp4' -and $_.FullName -notin $before}) {throw 'Video exported without consent'}
 $null=Invoke-App 'eval' @{script='(() => {const v = document.querySelector("video"); v.muted = true; document.querySelector("#play").click(); return true})()'}
 Start-Sleep -Milliseconds 650
 if (!(Invoke-App 'eval' @{script='document.querySelector("video").currentTime > 0'}).value) {throw 'Video did not play in preview'}
 $null=Invoke-App 'eval' @{script='(() => {const seek = document.querySelector("#seek"); document.querySelector("#play").click(); seek.value = 1.5; seek.dispatchEvent(new Event("input")); return true})()'}
 Start-Sleep -Milliseconds 400
 if (!(Invoke-App 'eval' @{script='(() => {const v = document.querySelector("video"); return v.paused && !v.seeking && Math.abs(v.currentTime - 1.5) < 0.15})()'}).value) {throw 'Video could not seek'}
 $null=Invoke-App 'eval' @{script='document.querySelector("#mark-out").click()'}
 if (!(Invoke-App 'eval' @{script='document.querySelector("#save").href.endsWith("?end=1.500")'}).value) {throw 'Trim marker did not update export range'}
 Capture 'recording-preview-verified.png'
 $null=Invoke-App 'eval' @{script='document.querySelector("#reset-trim").click()'}
 $previewHandle=[ScreenpinCaptureTest]::FindWindow([NullString]::Value,'录屏预览')
 $previewRect=[ScreenpinCaptureTest+Rect]::new()
 if (![ScreenpinCaptureTest]::GetWindowRect($previewHandle,[ref]$previewRect)) {throw 'Preview frame missing'}
 if (!$SkipPointerCheck) {
 [ScreenpinTestFocus]::SetForegroundWindow($previewHandle) | Out-Null
 Start-Sleep -Milliseconds 200
 $titleX=$previewRect.Left+260;$titleY=$previewRect.Top+20
 [ScreenpinCaptureTest]::SetCursorPos($titleX,$titleY) | Out-Null
 Start-Sleep -Milliseconds 100
 [ScreenpinCaptureTest]::mouse_event(0x0002,0,0,0,[UIntPtr]::Zero)
 try {
  Start-Sleep -Milliseconds 100
  [ScreenpinCaptureTest]::SetCursorPos($titleX+42,$titleY+24) | Out-Null
  Start-Sleep -Milliseconds 250
 } finally { [ScreenpinCaptureTest]::mouse_event(0x0004,0,0,0,[UIntPtr]::Zero) }
 Start-Sleep -Milliseconds 150
 $movedRect=[ScreenpinCaptureTest+Rect]::new()
 [ScreenpinCaptureTest]::GetWindowRect($previewHandle,[ref]$movedRect) | Out-Null
 if ($movedRect.Left -lt $previewRect.Left+20) {throw 'Preview title bar did not drag the native window'}
 $previewRect=$movedRect
 }

 if (!$SkipPointerCheck) {
 $desktopPreview=[System.Drawing.Bitmap]::new($previewRect.Right-$previewRect.Left,$previewRect.Bottom-$previewRect.Top)
 $desktopGraphics=[System.Drawing.Graphics]::FromImage($desktopPreview)
 try {
  $desktopGraphics.CopyFromScreen($previewRect.Left,$previewRect.Top,0,0,$desktopPreview.Size)
  $topPixel=$desktopPreview.GetPixel([int]($desktopPreview.Width/2),3)
  if (!$SkipPointerCheck -and ($topPixel.R -lt 175 -or $topPixel.G -lt 185 -or $topPixel.B -lt 190 -or $topPixel.B -lt $topPixel.R+3)) {throw "Preview title bar is outside the light palette ($topPixel)"}
  $desktopPreview.Save((Join-Path $artifactDir 'recording-preview-desktop.png'),[System.Drawing.Imaging.ImageFormat]::Png)
 } finally {$desktopGraphics.Dispose();$desktopPreview.Dispose()}
 }
 $null=Invoke-App 'eval' @{script='document.querySelector("a[href=maximize]").click()'}
 Start-Sleep -Milliseconds 250
 if (![ScreenpinCaptureTest]::IsZoomed($previewHandle)) {throw 'Preview maximize button did not maximize'}
 $null=Invoke-App 'eval' @{script='document.querySelector("a[href=maximize]").click()'}
 Start-Sleep -Milliseconds 250
 if ([ScreenpinCaptureTest]::IsZoomed($previewHandle)) {throw 'Preview maximize button did not restore'}
 $null=Invoke-App 'eval' @{script='document.querySelector("a.primary").click()'}
 Wait-PreviewSaved
 $timer=[Diagnostics.Stopwatch]::StartNew()
 do {
  $video=Get-ChildItem -LiteralPath $exportDirectory -ErrorAction SilentlyContinue -Filter '*.mp4' | Where-Object {$_.FullName -notin $before -and $_.Name -notlike '*.partial.mp4'} | Select-Object -First 1
  if ($video) {break}
  if ($timer.Elapsed.TotalSeconds -gt 15) {throw 'MP4 not saved'}
  Start-Sleep -Milliseconds 100
 } while ($true)
 $probe=& (Join-Path $projectDir 'tools/ffmpeg/ffprobe.exe') -v error -show_entries stream=codec_name,width,height,nb_frames -show_entries format=duration -of json $video.FullName | ConvertFrom-Json
 if ($probe.streams.Count -ne 2 -or $probe.streams[1].codec_name -ne 'aac') {throw 'System audio track missing from export'}
 $levels=& (Join-Path $projectDir 'tools/ffmpeg/ffmpeg.exe') -hide_banner -i $video.FullName -map '0:a:0' -af volumedetect -f null '-' 2>&1 | Out-String
 if ($levels -notmatch 'max_volume:\s*(-?[0-9.]+) dB' -or [double]$Matches[1] -lt -55) {throw "System audio track is silent: $levels"}
 if ($probe.streams[0].width -ne 318 -or $probe.streams[0].height -ne 240 -or [double]$probe.format.duration -le 1) {throw 'Wrong MP4 dimensions or duration'}
 $hashes=& (Join-Path $projectDir 'tools/ffmpeg/ffmpeg.exe') -v error -i $video.FullName -vf fps=2 -f framemd5 -
 if ($LASTEXITCODE -ne 0) {throw 'MP4 did not decode'}
 $unique=@($hashes | Where-Object {$_ -and !$_.StartsWith('#')} | ForEach-Object {($_ -split ',')[-1].Trim()} | Sort-Object -Unique)
 if ($unique.Count -lt 2) {throw 'Recorded frames did not change'}
 [ScreenpinCaptureTest]::SetWindowPos($previewHandle,[IntPtr]::Zero,0,0,550,420,0x16) | Out-Null
 Start-Sleep -Milliseconds 250
 $previewLayout='JSON.stringify({width:innerWidth,height:innerHeight,scrollWidth:document.documentElement.scrollWidth,scrollHeight:document.documentElement.scrollHeight,saveRight:document.querySelector("#save").getBoundingClientRect().right,saveBottom:document.querySelector("#save").getBoundingClientRect().bottom,videoHeight:document.querySelector("video").clientHeight, footerHeight:document.querySelector("#transport").clientHeight, fits:document.documentElement.scrollWidth <= innerWidth && document.documentElement.scrollHeight <= innerHeight && document.querySelector("#save").getBoundingClientRect().right <= innerWidth && document.querySelector("#save").getBoundingClientRect().bottom <= innerHeight})'
 $layoutBefore=(Invoke-App 'eval' @{script=$previewLayout}).value | ConvertFrom-Json
 $null=Invoke-App 'eval' @{script='document.querySelector("video").currentTime=0.25; document.querySelector("#mark-in").click(); document.querySelector("video").currentTime=1.25; document.querySelector("#mark-out").click(); document.querySelector("#save").click()'}
 $trimTimer=[Diagnostics.Stopwatch]::StartNew()
 do {
  $trimmed=Get-ChildItem -LiteralPath $exportDirectory -ErrorAction SilentlyContinue -Filter '*_裁剪.mp4' | Where-Object {$_.FullName -notin $before} | Select-Object -First 1
  if (!$trimmed) {Start-Sleep -Milliseconds 100}
  if ($trimTimer.Elapsed.TotalSeconds -gt 15) {
   $trimStatus=(Invoke-App 'eval' @{script='document.querySelector("#status").textContent'}).value
   throw "Trimmed file not exported from preview: $trimStatus"
  }
 } until ($trimmed)
 Wait-PreviewSaved
 $layoutAfter=(Invoke-App 'eval' @{script=$previewLayout}).value | ConvertFrom-Json
 if (!$layoutBefore.fits -or !$layoutAfter.fits -or $layoutBefore.videoHeight -ne $layoutAfter.videoHeight -or $layoutBefore.footerHeight -ne $layoutAfter.footerHeight) {
  throw "Trim save changed the preview layout: before=$layoutBefore after=$layoutAfter"
 }
 $trimLength=& (Join-Path $projectDir 'tools/ffmpeg/ffprobe.exe') -v error -show_entries format=duration -of csv=p=0 $trimmed.FullName
 if ([double]$trimLength -lt 0.9 -or [double]$trimLength -gt 1.12) {throw "Wrong preview trim duration: $trimLength"}
 $null=Invoke-App 'eval' @{script='document.querySelector("a[href=close]").click()'}
 Copy-Item -LiteralPath $video.FullName -Destination (Join-Path $artifactDir 'recording-demo.mp4')
 Show-TestPanel;Main-Window
 Capture 'control-panel-media-verified.png'
 if ((Invoke-App 'tree').nodes.label -notcontains '打开视频') {throw 'Recording playback entry missing'}
 $beforeQuit=@(Get-ChildItem -LiteralPath $exportDirectory -ErrorAction SilentlyContinue -Filter '*.mp4' | Where-Object {$_.Name -notlike '*.partial.mp4'}).FullName
 [ScreenpinPatternSource]::Front()
 Start-RecordingTest;Select-Pattern -fullDesktop
 $timer=[Diagnostics.Stopwatch]::StartNew()
 do {
  $task=@((Invoke-App 'windows').windows) | Where-Object {$_.title -eq '区域录屏'} | Select-Object -First 1
  if ($task){$script:windowId=$task.id;Settle;$labels=(Invoke-App 'tree').nodes.label}
  if (($labels -join ' ') -match '录屏中') {break}
  if ($timer.Elapsed.TotalSeconds -gt 10) {throw 'Second recording did not begin'}
  Start-Sleep -Milliseconds 100
 } while ($true)
 $control=[ScreenpinCaptureTest]::FindWindow([NullString]::Value,'区域录屏')
 $outline=[ScreenpinCaptureTest]::FindWindow('ScreenpinRegionOutline','Screenpin 选区边框')
 foreach ($handle in @($control,$outline)) {
  $affinity=[uint32]0
  if ($handle -eq [IntPtr]::Zero -or ![ScreenpinCaptureTest]::IsWindowVisible($handle) -or ![ScreenpinCaptureTest]::GetWindowDisplayAffinity($handle,[ref]$affinity) -or $affinity -ne 17) {throw "Full-desktop recording UI is missing or not excluded from capture (control=$control, outline=$outline, current=$handle, affinity=$affinity)"}
 }
 Show-TestPanel;Main-Window;Settle
 $exit=(Invoke-App 'tree').nodes | Where-Object {$_.label -eq '退出程序' -and $_.actions -contains 'press'} | Select-Object -First 1
 try {Invoke-App 'act' @{id=$exit.id;action='press'} | Out-Null} catch {if (!$process.HasExited){throw}}
 Wait-RecordPreview
 if (Get-ChildItem -LiteralPath $exportDirectory -ErrorAction SilentlyContinue -Filter '*.mp4' | Where-Object {$_.Name -notlike '*.partial.mp4' -and $_.FullName -notin $beforeQuit}) {throw 'Video exported while quitting without consent'}
 $null=Invoke-App 'eval' @{script='document.querySelector("a.primary").click()'}
 Wait-PreviewSaved
 try {$null=Invoke-App 'eval' @{script='document.querySelector("a[href=close]").click()'}} catch {if (!$process.HasExited){throw}}
 if (!$process.WaitForExit(15000)){throw 'Application did not exit after closing the saved preview'}
 $quitVideo=Get-ChildItem -LiteralPath $exportDirectory -ErrorAction SilentlyContinue -Filter '*.mp4' | Where-Object {$_.Name -notlike '*.partial.mp4' -and $_.FullName -notin $beforeQuit} | Select-Object -First 1
 if (!$quitVideo) {throw 'Exit lost the active recording'}
 $quitProbe=& (Join-Path $projectDir 'tools/ffmpeg/ffprobe.exe') -v error -show_entries stream=codec_name,width,height,nb_frames -of json $quitVideo.FullName | ConvertFrom-Json
 if ($LASTEXITCODE -ne 0 -or [int]$quitProbe.streams[0].nb_frames -lt 1) {throw 'Recording saved on exit was not playable'}
 if (!$RecordingOnly) { Write-Output "PASS: exact PNG pixels, automatic temporary pin, manual resident pin from clipboard pixels and a copied PNG file, and 160x380 exact scroll stitch." }
 Write-Output "PASS: animated 318x240 H.264 recording ($($probe.streams[0].nb_frames) frames), preview playback and explicit export before exit."
} finally {
 [ScreenpinPatternSource]::Stop()
 $stream.Dispose()
 if (!$process.HasExited){Stop-Process -Id $process.Id}
}
