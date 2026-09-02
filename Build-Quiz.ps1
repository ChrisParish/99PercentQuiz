<#
.SYNOPSIS
    Merges the per-category question files into the working pool and the delivered file.

.DESCRIPTION
    Reads working/categories/*.json, drops reserve questions, spreads every category
    evenly across the running order so any 20-question day is varied, then writes:
      working/pool.json  - full records (id, category, acceptedAnswers)
      questions.json     - question and answer only, as the brief specifies

.EXAMPLE
    .\Build-Quiz.ps1
#>
[CmdletBinding()]
param(
    [string]$CategoryDir = (Join-Path $PSScriptRoot 'working\categories'),
    [string]$PoolPath = (Join-Path $PSScriptRoot 'working\pool.json'),
    [string]$OutPath = (Join-Path $PSScriptRoot 'questions.json'),

    # Fixed seed keeps the running order reproducible between builds.
    [int]$Seed = 99
)

$ErrorActionPreference = 'Stop'

$files = Get-ChildItem -LiteralPath $CategoryDir -Filter '*.json' | Sort-Object Name
if (-not $files) { throw "No category files found in $CategoryDir" }

$all = New-Object System.Collections.Generic.List[object]
foreach ($file in $files) {
    $loaded = Get-Content -LiteralPath $file.FullName -Raw -Encoding UTF8 | ConvertFrom-Json
    foreach ($item in $loaded) { $all.Add($item) | Out-Null }
    Write-Host ("  {0,-18} {1,3} questions" -f $file.BaseName, @($loaded).Count)
}

$main = @($all | Where-Object { -not $_.reserve })
$reserve = @($all | Where-Object { $_.reserve })
Write-Host ''
Write-Host "Main set: $($main.Count)   Reserve: $($reserve.Count)" -ForegroundColor Cyan

# Spread each category evenly: position a question by how far through its own
# category it sits, so a window of 20 draws proportionally from every category.
$rng = New-Object System.Random($Seed)
$ordered = New-Object System.Collections.Generic.List[object]

foreach ($group in ($main | Group-Object -Property category | Sort-Object Name)) {
    $shuffled = $group.Group | Sort-Object { $rng.Next() }
    $n = @($shuffled).Count
    for ($i = 0; $i -lt $n; $i++) {
        $ordered.Add([pscustomobject]@{
                Position = ($i + 0.5) / $n
                Category = $group.Name
                Item     = $shuffled[$i]
            }) | Out-Null
    }
}

$sorted = $ordered | Sort-Object Position, Category
$final = $sorted | ForEach-Object { $_.Item }

$poolJson = ($final + $reserve) | ConvertTo-Json -Depth 5
Set-Content -LiteralPath $PoolPath -Value $poolJson -Encoding UTF8

$delivered = $final | Select-Object -Property question, answer
$outJson = $delivered | ConvertTo-Json -Depth 3
Set-Content -LiteralPath $OutPath -Value $outJson -Encoding UTF8

Write-Host ''
Write-Host "Wrote $PoolPath ($(@($final).Count) + $($reserve.Count) reserve)" -ForegroundColor Green
Write-Host "Wrote $OutPath ($(@($delivered).Count) questions)" -ForegroundColor Green

Write-Host ''
Write-Host 'Category spread per broadcast day:' -ForegroundColor Cyan
for ($d = 0; $d + 20 -le $final.Count; $d += 20) {
    $block = $final[$d..($d + 19)]
    $cats = ($block | Select-Object -ExpandProperty category -Unique | Sort-Object) -join ', '
    Write-Host ("  Day {0,2}: {1}" -f ([int]($d / 20) + 1), $cats) -ForegroundColor DarkGray
}
