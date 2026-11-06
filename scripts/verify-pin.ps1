param([string]$AppFile = 'artifacts/screenpin-check.exe')
$ErrorActionPreference = 'Stop'
$projectDir = Split-Path $PSScriptRoot -Parent
$artifactDir = Join-Path $projectDir 'artifacts'
New-Item -ItemType Directory -Force -Path $artifactDir | Out-Null
$env:APPDATA = Join-Path $env:TEMP ('screenpin-settings-test-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $env:APPDATA | Out-Null
$env:MYGO_LOCALE = 'zh-Hans'
$env:SCREENPIN_VIDEO_DIR = Join-Path $artifactDir 'media-validation'
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
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern IntPtr FindWindow(string className,string title);
 [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr window,uint message,IntPtr wparam,IntPtr lparam);
 [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr window);
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
 static int offset;
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
  ready.Reset();offset=0;
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
 public static void SetOffset(int value){SendMessage(Handle,0x8008,new IntPtr(value),IntPtr.Zero);}
 public static void Wheel(){PostMessage(Handle,0x20a,new IntPtr(-120<<16),IntPtr.Zero);}
 public static void Stop(){if(Handle!=IntPtr.Zero){PostMessage(Handle,0x10,IntPtr.Zero,IntPtr.Zero);thread.Join(3000);Handle=IntPtr.Zero;}}
 static IntPtr OnMessage(IntPtr h,uint m,IntPtr w,IntPtr l){
  if(m==0x14)return new IntPtr(1);
  if(m==0x8008||m==0x20a){offset=m==0x8008?w.ToInt32():Math.Min(1000,offset+45);InvalidateRect(h,IntPtr.Zero,false);UpdateWindow(h);return IntPtr.Zero;}
  if(m==0xf){
   Paint paint;IntPtr dc=BeginPaint(h,out paint);byte[] pixels=new byte[360*380*4];
   for(int y=0;y<380;y++)for(int x=0;x<360;x++){int row=y+offset,i=(y*360+x)*4;pixels[i]=(byte)((x*row*7+row*23)%256);pixels[i+1]=(byte)((x*43+row*row*3)%256);pixels[i+2]=(byte)((x*13+row*37)%256);pixels[i+3]=255;}
   var info=new Info{Size=40,Width=360,Height=-380,Planes=1,Bits=32};SetDIBitsToDevice(dc,0,0,360,380,0,0,0,380,pixels,ref info,0);EndPaint(h,ref paint);return IntPtr.Zero;
  }
  if(m==0x10){DestroyWindow(h);return IntPtr.Zero;}if(m==2){PostQuitMessage(0);return IntPtr.Zero;}
  return DefWindowProc(h,m,w,l);
 }
}
'@

function Select-Pattern($width=160,$height=200) {
 $overlay=Wait-Overlay
 $left=410-[ScreenpinCaptureTest]::GetSystemMetrics(76)
 $top=210-[ScreenpinCaptureTest]::GetSystemMetrics(77)
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
function Wait-Pin($present) {
 $timer=[Diagnostics.Stopwatch]::StartNew()
 do {
  $pin=@((Invoke-App 'windows').windows) | Where-Object {$_.title -like '贴图*'} | Select-Object -First 1
  if (($null -ne $pin) -eq $present) {return $pin}
  if ($timer.Elapsed.TotalSeconds -gt 5) {throw "Pin presence did not become $present"}
  Start-Sleep -Milliseconds 50
 } while ($true)
}


Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class ScreenpinPinPointer {
 [DllImport("user32.dll")] public static extern bool SetCursorPos(int x,int y);
 [DllImport("user32.dll")] public static extern void mouse_event(uint flags,uint x,uint y,uint data,UIntPtr extra);
}
'@

try {
 Capture 'control-panel-media.png'
 Click-Label '设置'
 Fill-Key '区域截图快捷键' 'Ctrl+Alt+7'
 Fill-Key '剪贴板图片贴图快捷键' 'Ctrl+Alt+8'
 Fill-Key '显示面板快捷键' 'Ctrl+Alt+9'
 Fill-Key '滚动截图 / 完成快捷键' 'Ctrl+Alt+5'
 Fill-Key '录屏 / 停止快捷键' 'Ctrl+Alt+6'
 Click-Label '应用快捷键'
 if ((Invoke-App 'tree').nodes.label -notcontains '快捷键已应用并保存') {throw 'Test shortcuts did not apply'}
 Click-Label '工作台';Click-Label '收起到托盘'
 [ScreenpinPatternSource]::Start()
 Start-Sleep -Milliseconds 300
 $reference=[System.Drawing.Bitmap]::new(1,1);$graphics=[System.Drawing.Graphics]::FromImage($reference)
 $graphics.CopyFromScreen(420,220,0,0,$reference.Size);$pixel=$reference.GetPixel(0,0);$graphics.Dispose();$reference.Dispose()
 if ($pixel.R -ne ((20*13+20*37)%256)) {throw "Desktop pattern source not visible: $pixel"}
 $sequence=[ScreenpinCaptureTest]::GetClipboardSequenceNumber()
 Send-Hotkey 0x37;Select-Pattern
 $bytes=Read-NewPNG $sequence;Assert-Pattern $bytes 200
 $pin=Wait-Pin $true
 $script:windowId=$pin.id
 $labels=(Invoke-App 'tree').nodes.label
 foreach ($unwanted in @('常驻','1:1','关闭','隐藏','临时贴图 · 点击窗口外关闭','已常驻 · Esc 关闭')) {if ($labels -contains $unwanted){throw "Unwanted pin chrome: $unwanted"}}
 Capture 'pin-image-only.png'
 $render=[System.Drawing.Bitmap]::new((Join-Path $artifactDir 'pin-image-only.png'))
 try {
  if ($render.Width -ne 160 -or $render.Height -ne 200){throw "Pin does not fit image: $($render.Size)"}
  foreach ($corner in @(@(0,0),@(($render.Width-1),0),@(0,($render.Height-1)),@(($render.Width-1),($render.Height-1)))) {if ($render.GetPixel($corner[0],$corner[1]).A -gt 5){throw 'Pin has an opaque square corner'}}
 } finally {$render.Dispose()}
 $pinHandle=[ScreenpinPatternSource]::FindTitle($pin.title)
 $beforeDrag=[ScreenpinCaptureTest+Rect]::new()
 [ScreenpinCaptureTest]::GetWindowRect($pinHandle,[ref]$beforeDrag) | Out-Null
 [ScreenpinPinPointer]::SetCursorPos($beforeDrag.Left+40,$beforeDrag.Top+80) | Out-Null
 [ScreenpinPinPointer]::mouse_event(2,0,0,0,[UIntPtr]::Zero)
 Start-Sleep -Milliseconds 150
 [ScreenpinPinPointer]::SetCursorPos($beforeDrag.Left+88,$beforeDrag.Top+116) | Out-Null
 Start-Sleep -Milliseconds 150
 [ScreenpinPinPointer]::mouse_event(4,0,0,0,[UIntPtr]::Zero)
 Start-Sleep -Milliseconds 200
 $afterDrag=[ScreenpinCaptureTest+Rect]::new()
 [ScreenpinCaptureTest]::GetWindowRect($pinHandle,[ref]$afterDrag) | Out-Null
 if ([Math]::Abs($afterDrag.Left-$beforeDrag.Left-48) -gt 3 -or [Math]::Abs($afterDrag.Top-$beforeDrag.Top-36) -gt 3) {throw "Dragging the image did not move the window: $beforeDrag -> $afterDrag"}
 [ScreenpinPatternSource]::Front();$null=Wait-Pin $false
 Send-Hotkey 0x38
 $pin=Wait-Pin $true
 [ScreenpinPatternSource]::Front();Start-Sleep -Milliseconds 250
 $null=Wait-Pin $true
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

 Send-Hotkey 0x39;Main-Window;Settle
 $exit=(Invoke-App 'tree').nodes | Where-Object {$_.label -eq '退出程序' -and $_.actions -contains 'press'} | Select-Object -First 1
 try {Invoke-App 'act' @{id=$exit.id;action='press'} | Out-Null} catch {if (!$process.HasExited){throw}}
 if (!$process.WaitForExit(5000)){throw 'Application did not exit'}
 Write-Output 'PASS: image-only rounded pin, no title/footer/window buttons, native image dragging, manual resident pin and click-outside dismissal.'
} finally {
 [ScreenpinPatternSource]::Stop()
 $stream.Dispose()
 if (!$process.HasExited){Stop-Process -Id $process.Id}
}
