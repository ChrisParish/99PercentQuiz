<#
.SYNOPSIS
    Emits blind fact-check batches for a second model to answer.

.DESCRIPTION
    Writes numbered prompt files containing questions with their answers removed.
    Paste each file into a chat with a DIFFERENT model from the one that authored
    the questions, then save the JSON reply to working/factcheck/reply-NN.json.

    Merge the replies into one array and feed it back in:
        .\Test-Quiz.ps1 -Path .\working\pool.json -FactCheckPath .\working\factcheck\merged.json

.EXAMPLE
    .\New-FactCheckBatches.ps1
    .\New-FactCheckBatches.ps1 -BatchSize 25
#>
[CmdletBinding()]
param(
    [string]$PoolPath = (Join-Path $PSScriptRoot 'working\pool.json'),
    [string]$OutDir = (Join-Path $PSScriptRoot 'working\factcheck'),
    [int]$BatchSize = 50
)

$ErrorActionPreference = 'Stop'

if (-not (Test-Path -LiteralPath $OutDir)) { New-Item -ItemType Directory -Path $OutDir | Out-Null }
Get-ChildItem -LiteralPath $OutDir -Filter 'batch-*.md' -ErrorAction SilentlyContinue | Remove-Item

$items = Get-Content -LiteralPath $PoolPath -Raw -Encoding UTF8 | ConvertFrom-Json

$instructions = @'
You are fact-checking questions for a UK breakfast-radio quiz aimed at the general public.

Answer each question from your own knowledge. The quiz's own answers have been
withheld deliberately - do not try to guess what they were, just answer honestly.

For each question return:
  answer     - your answer, 4 words maximum, digits for numbers (45, not forty-five)
  difficulty - 1 to 5, judging what share of UK adults would answer correctly:
               1 = virtually all (99%+)   2 = most   3 = around half
               4 = a minority             5 = specialist knowledge
  ambiguous  - true if more than one answer is genuinely defensible
  reason     - short note, only when ambiguous is true, otherwise ""

Use UK English and UK conventions throughout.

Reply with ONLY a JSON array. No commentary before or after:

[{"id":"example-001","answer":"45","difficulty":1,"ambiguous":false,"reason":""}]
'@

$batchNumber = 0
for ($i = 0; $i -lt $items.Count; $i += $BatchSize) {
    $batchNumber++
    $end = [Math]::Min($i + $BatchSize - 1, $items.Count - 1)
    $batch = $items[$i..$end]

    $lines = New-Object System.Collections.Generic.List[string]
    $lines.Add("# Fact-check batch $batchNumber") | Out-Null
    $lines.Add('') | Out-Null
    $lines.Add($instructions) | Out-Null
    $lines.Add('') | Out-Null
    $lines.Add('## Questions') | Out-Null
    $lines.Add('') | Out-Null
    foreach ($item in $batch) {
        $lines.Add("$($item.id) | $($item.question)") | Out-Null
    }

    $path = Join-Path $OutDir ('batch-{0:D2}.md' -f $batchNumber)
    Set-Content -LiteralPath $path -Value ($lines -join [Environment]::NewLine) -Encoding UTF8
    Write-Host ("Wrote {0} ({1} questions)" -f $path, @($batch).Count)
}

Write-Host ''
Write-Host "$batchNumber batches covering $($items.Count) questions." -ForegroundColor Green
Write-Host 'Answer each batch with a different model, save replies to this folder, then merge and run:' -ForegroundColor Cyan
Write-Host '  .\Test-Quiz.ps1 -Path .\working\pool.json -FactCheckPath .\working\factcheck\merged.json' -ForegroundColor Cyan
