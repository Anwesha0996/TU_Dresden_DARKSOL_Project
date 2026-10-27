param([switch]$KeepPreview, [switch]$HtmlOnly)

$ErrorActionPreference = 'Stop'
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$tex = Join-Path $here 'main.tex'
$pdf = Join-Path $here 'main.pdf'

$pdflatex = Get-Command pdflatex -ErrorAction SilentlyContinue
if ($pdflatex) {
    Push-Location $here
    try {
        & $pdflatex.Source -interaction=nonstopmode -halt-on-error main.tex
        & $pdflatex.Source -interaction=nonstopmode -halt-on-error main.tex
    } finally {
        Pop-Location
    }
    exit $LASTEXITCODE
}

# Portable preview fallback for machines without TeX. main.tex remains the
# authoritative source; this transforms its body into HTML before PDF printing.
$source = Get-Content -LiteralPath $tex -Raw
$body = [regex]::Match($source, '(?s)\\begin\{document\}(.*)\\end\{document\}').Groups[1].Value
$body = [System.Net.WebUtility]::HtmlEncode($body)
$body = [regex]::Replace($body, '(?s)\\begin\{lstlisting\}.*?\n(.*?)\\end\{lstlisting\}', '<pre>$1</pre>')
$body = [regex]::Replace($body, '(?s)\\begin\{verbatim\}(.*?)\\end\{verbatim\}', '<pre>$1</pre>')
$body = [regex]::Replace($body, '\\section\{([^}]*)\}', '<h1>$1</h1>')
$body = [regex]::Replace($body, '\\subsection\{([^}]*)\}', '<h2>$1</h2>')
$body = [regex]::Replace($body, '\\title\{.*?\}|\\author\{.*?\}|\\date\{.*?\}|\\maketitle|\\tableofcontents|\\newpage', '')
$body = [regex]::Replace($body, '\\file\{([^}]*)\}', '<code>$1</code>')
$body = [regex]::Replace($body, '\\lines\{([^}]*)\}', 'lines $1')
$body = [regex]::Replace($body, '\\texttt\{([^}]*)\}', '<code>$1</code>')
$body = [regex]::Replace($body, '\\emph\{([^}]*)\}', '<em>$1</em>')
$body = $body -replace '\\begin\{abstract\}', '<div class="abstract"><h1>Abstract</h1>' -replace '\\end\{abstract\}', '</div>'
$body = $body -replace '\\begin\{enumerate\}(\[.*?\])?', '<ol>' -replace '\\end\{enumerate\}', '</ol>'
$body = $body -replace '\\begin\{itemize\}(\[.*?\])?', '<ul>' -replace '\\end\{itemize\}', '</ul>'
$body = $body -replace '\\item\s*', '<li>'
$body = $body -replace '\\begin\{center\}', '<p class="center">' -replace '\\end\{center\}', '</p>'
$body = $body -replace '\\rightarrow', '&rarr;' -replace '\\mu', '&mu;' -replace '\\times', '&times;'
$body = $body -replace '\\\(', '' -replace '\\\)', '' -replace '\\%', '%' -replace '\\&', '&amp;'
$body = [regex]::Replace($body, '(?s)\\begin\{longtable\}.*?\\end\{longtable\}', '<p><em>Component and modification summaries are explained in the surrounding sections.</em></p>')
$body = $body -replace '\\begin\{lstlisting\}[^\n]*', '<pre>' -replace '\\end\{lstlisting\}', '</pre>'
$body = $body -replace '\\begin\{[^}]+\}|\\end\{[^}]+\}|\\toprule|\\midrule|\\bottomrule', ''
$body = $body -replace '\\\\', '<br>'
$body = $body -replace '\{', '' -replace '\}', '' -replace '---', '&mdash;'
$paragraphs = [regex]::Split($body, '(\r?\n){2,}') | Where-Object { $_.Trim() }
$body = ($paragraphs | ForEach-Object {
    $p = $_.Trim()
    if ($p -match '^<(h1|h2|ol|ul|li|pre|div|p)') { $p } else { '<p>' + ($p -replace '\r?\n', ' ') + '</p>' }
}) -join "`n"

$html = @"
<!doctype html><html><head><meta charset="utf-8"><title>Helm-DTN Standalone OCI Image Transfer</title>
<style>@page{size:A4;margin:18mm 17mm 19mm}body{font:11pt/1.48 Georgia,serif;color:#17202a;max-width:920px;margin:auto}h1{font:700 20pt Arial,sans-serif;color:#163a5f;border-bottom:1px solid #9fb3c8;padding-bottom:5px;page-break-after:avoid}h2{font:700 14pt Arial,sans-serif;color:#24557a;page-break-after:avoid}code,pre{font-family:Consolas,monospace;background:#f3f5f7}code{padding:1px 3px}pre{padding:10px;border:1px solid #ccd5dd;white-space:pre-wrap;page-break-inside:avoid}.title{text-align:center;margin:80px 0 50px}.title h1{font-size:27pt;border:0}.abstract{background:#f5f8fb;padding:12px 18px}.center{text-align:center;font-weight:bold}li{margin:5px 0}p{text-align:justify}</style></head><body>
<div class="title"><h1>Extending Helm-DTN for Standalone OCI Image Transfer</h1><h2>Beginner-Oriented Design and Code Walkthrough</h2><p>Implementation report &mdash; 5 August 2026</p></div>
$body
</body></html>
"@
$preview = Join-Path $here ("main-preview-{0}.html" -f $PID)
Set-Content -LiteralPath $preview -Value $html -Encoding UTF8
if ($HtmlOnly) {
    Write-Output $preview
    exit 0
}

$word = $null
$document = $null
try {
    $word = New-Object -ComObject Word.Application
    $word.Visible = $false
    $word.DisplayAlerts = 0
    $document = $word.Documents.Open($preview, $false, $true)
    # 17 is Word's wdExportFormatPDF value.
    if (Test-Path -LiteralPath $pdf) { Remove-Item -LiteralPath $pdf }
    $document.ExportAsFixedFormat($pdf, 17)
    $pageCount = $document.ComputeStatistics(2)
    $characterCount = $document.ComputeStatistics(3)
} finally {
    if ($document) { $document.Close(0) }
    if ($word) { $word.Quit() }
}
if (-not (Test-Path -LiteralPath $pdf) -or (Get-Item -LiteralPath $pdf).Length -eq 0) {
    throw 'PDF generation failed.'
}
if ($pageCount -lt 1 -or $characterCount -lt 1000) {
    throw "PDF content validation failed: pages=$pageCount characters=$characterCount"
}
if (-not $KeepPreview) { Remove-Item -LiteralPath $preview }
Write-Output "Generated $pdf from $tex ($pageCount pages, $characterCount characters)"
