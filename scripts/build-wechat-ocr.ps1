param(
    [string]$DotNet = 'dotnet',
    [string]$ComponentDirectory = (Join-Path $env:LOCALAPPDATA 'STranslate/current/Plugins/STranslate.Plugin.Ocr.WeChatBuiltIn')
)

$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$componentPath = (Resolve-Path -LiteralPath $ComponentDirectory).Path
$assembly = Join-Path $componentPath 'WeChatOcr.dll'
if (!(Test-Path -LiteralPath $assembly -PathType Leaf)) {
    throw '找不到 WeChatOcr.dll，请指定 STranslate 的内置微信 OCR 插件目录。'
}

$project = Join-Path $root 'tools/wechat-ocr-src/Screenpin.WeChatOcr.csproj'
$output = Join-Path $root 'tools/wechat-ocr'
& $DotNet publish $project -c Release -r win-x64 --self-contained false "-p:WeChatOcrDirectory=$componentPath" -o $output
if ($LASTEXITCODE -ne 0) {exit $LASTEXITCODE}
