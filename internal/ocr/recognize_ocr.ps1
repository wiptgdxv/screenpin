$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$null = [Windows.Storage.StorageFile, Windows.Storage, ContentType=WindowsRuntime]
$null = [Windows.Storage.Streams.IRandomAccessStream, Windows.Storage.Streams, ContentType=WindowsRuntime]
$null = [Windows.Graphics.Imaging.BitmapDecoder, Windows.Graphics.Imaging, ContentType=WindowsRuntime]
$null = [Windows.Graphics.Imaging.SoftwareBitmap, Windows.Graphics.Imaging, ContentType=WindowsRuntime]
$null = [Windows.Media.Ocr.OcrEngine, Windows.Foundation, ContentType=WindowsRuntime]
$null = [Windows.Media.Ocr.OcrResult, Windows.Foundation, ContentType=WindowsRuntime]
$asTask = [System.WindowsRuntimeSystemExtensions].GetMethods().Where({
    $_.Name -eq 'AsTask' -and $_.IsGenericMethod -and $_.GetParameters().Count -eq 1 -and
    $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1'
})[0]
function Await($operation, [type] $resultType) {
    $task = $asTask.MakeGenericMethod($resultType).Invoke($null, @($operation))
    $task.GetAwaiter().GetResult()
}
if ($env:SCREENPIN_OCR_LANGUAGE) {
    $null = [Windows.Globalization.Language, Windows.Globalization, ContentType=WindowsRuntime]
    $language = [Windows.Globalization.Language]::new($env:SCREENPIN_OCR_LANGUAGE)
    $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($language)
} else {
    $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()
    if ($null -eq $engine) {
        $language = [Windows.Media.Ocr.OcrEngine]::AvailableRecognizerLanguages | Select-Object -First 1
        if ($null -ne $language) { $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($language) }
    }
}
if ($null -eq $engine) { [Console]::Write('!language'); exit }
$chinese = $english = $null
if (!$env:SCREENPIN_OCR_LANGUAGE) {
    $null = [Windows.Globalization.Language, Windows.Globalization, ContentType=WindowsRuntime]
    if ($engine.RecognizerLanguage.LanguageTag -notlike 'zh-*') {
        $chinese = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage([Windows.Globalization.Language]::new('zh-Hans-CN'))
    }
    if ($engine.RecognizerLanguage.LanguageTag -notlike 'en-*') {
        $english = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage([Windows.Globalization.Language]::new('en-US'))
    }
}

function Get-Lines($result, $tile) {
    foreach ($line in $result.Lines) {
        if ($line.Words.Count -eq 0) { continue }
        $left = $top = [double]::PositiveInfinity
        $bottom = [double]::NegativeInfinity
        foreach ($word in $line.Words) {
            $rect = $word.BoundingRect
            $left = [Math]::Min($left, $rect.X)
            $top = [Math]::Min($top, $rect.Y)
            $bottom = [Math]::Max($bottom, $rect.Y + $rect.Height)
        }
        # Each tile owns lines centered in its unshared area.
        $center = ($top + $bottom) / 2
        if ($tile.KeepTo -gt 0 -and ($center -lt $tile.KeepFrom -or $center -ge $tile.KeepTo)) { continue }
        [pscustomobject]@{ Text = $line.Text; X = $left; Y = $center; Height = $bottom - $top }
    }
}

$tiles = Get-Content -LiteralPath $env:SCREENPIN_OCR_MANIFEST -Raw -Encoding UTF8 | ConvertFrom-Json
$lines = foreach ($tile in $tiles) {
    $file = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($tile.File)) ([Windows.Storage.StorageFile])
    $stream = Await ($file.OpenAsync([Windows.Storage.FileAccessMode]::Read)) ([Windows.Storage.Streams.IRandomAccessStream])
    try {
        $decoder = Await ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
        $bitmap = Await ($decoder.GetSoftwareBitmapAsync()) ([Windows.Graphics.Imaging.SoftwareBitmap])
        try {
            $result = Await ($engine.RecognizeAsync($bitmap)) ([Windows.Media.Ocr.OcrResult])
            $primaryLines = @(Get-Lines $result $tile)
            $selectedLines = $primaryLines
            $text = $primaryLines.Text -join [Environment]::NewLine
            $useChinese = $engine.RecognizerLanguage.LanguageTag -like 'zh-*'
            if ($null -ne $chinese) {
                $candidate = Await ($chinese.RecognizeAsync($bitmap)) ([Windows.Media.Ocr.OcrResult])
                $chineseLines = @(Get-Lines $candidate $tile)
                $candidateText = $chineseLines.Text -join [Environment]::NewLine
                if ($candidateText -match '\p{IsCJKUnifiedIdeographs}[ \t]*\p{IsCJKUnifiedIdeographs}' -or (!$text -and $candidateText)) {
                    $selectedLines = $chineseLines
                    $useChinese = $true
                }
            }
            if ($useChinese -and !$env:SCREENPIN_OCR_LANGUAGE) {
                $englishLines = @()
                if ($engine.RecognizerLanguage.LanguageTag -like 'en-*') {
                    $englishLines = $primaryLines
                } elseif ($english) {
                    $latinResult = Await ($english.RecognizeAsync($bitmap)) ([Windows.Media.Ocr.OcrResult])
                    $englishLines = @(Get-Lines $latinResult $tile)
                }
                foreach ($line in $selectedLines) {
                    if ($line.Text -match '\p{IsCJKUnifiedIdeographs}') { continue }
                    $aligned = $englishLines | Where-Object {
                        [Math]::Abs($_.Y - $line.Y) -lt $line.Height / 2 -and
                        [Math]::Abs($_.X - $line.X) -lt $line.Height -and $_.Text -match '[A-Za-z]'
                    } | Select-Object -First 1
                    if ($aligned) { $line.Text = $aligned.Text }
                }
            }
            $text = $selectedLines.Text -join [Environment]::NewLine
            if ($useChinese) {
                $text = [regex]::Replace($text, '(?<=\p{IsCJKUnifiedIdeographs})[ \t]+(?=\p{IsCJKUnifiedIdeographs})', '')
            }
            if ($text) { $text }
        } finally { $bitmap.Dispose() }
    } finally { $stream.Dispose() }
}
[Console]::Write($lines -join [Environment]::NewLine)
