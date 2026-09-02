<#
.SYNOPSIS
    Deterministic validator for The 99% Quiz question set.

.DESCRIPTION
    Enforces the rules in rules.md. Runs against either the working pool
    (working/pool.json, extra fields allowed) or the delivered file
    (questions.json, -Strict). Exits 1 if any hard failure is found.

.EXAMPLE
    .\Test-Quiz.ps1 -Path .\working\pool.json
    .\Test-Quiz.ps1 -Path .\questions.json -Strict
    .\Test-Quiz.ps1 -Path .\working\pool.json -FactCheckPath .\working\factcheck-merged.json
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Path,

    # Delivered-file mode: require exactly 400 items with only question/answer.
    [switch]$Strict,

    # Optional blind second-model results to fold in.
    [string]$FactCheckPath,

    [int]$ExpectedCount = 400
)

$ErrorActionPreference = 'Stop'

$script:Failures = New-Object System.Collections.Generic.List[string]
$script:Warnings = New-Object System.Collections.Generic.List[string]

function Add-Failure { param([string]$Message) $script:Failures.Add($Message) | Out-Null }
function Add-Warning { param([string]$Message) $script:Warnings.Add($Message) | Out-Null }

function Get-NormalisedText {
    param([string]$Text)
    if ($null -eq $Text) { return '' }
    $t = $Text.ToLowerInvariant()
    $t = [regex]::Replace($t, '[^a-z0-9 ]', ' ')
    $t = [regex]::Replace($t, '\s+', ' ')
    return $t.Trim()
}

# --- Rule lists (mirror rules.md) -------------------------------------------

$BareSuperlatives = @(
    'smallest', 'largest', 'biggest', 'fastest', 'slowest', 'best', 'worst',
    'most', 'least', 'tallest', 'oldest', 'longest', 'strongest', 'highest', 'deepest'
)

# Rule 1 escape hatch: uncontested, dimension-explicit questions.
$SuperlativeAllowlist = @(
    'what is the largest planet in our solar system',
    'what is the largest ocean on earth',
    'what is the largest country in the world by area',
    'what is the largest mammal on earth',
    'what is the longest river in the world',
    'what is the tallest mountain in the world',
    'what is the largest desert in the world',
    'what is the largest continent by area',
    'which is the largest of the great lakes',
    'what is the most common blood type in the uk'
)

$Americanisms = @(
    'zucchini', 'eggplant', 'cilantro', 'soccer', 'sidewalk', 'elevator',
    'apartment', 'candy', 'diaper', 'gasoline', 'aluminum', 'cookie',
    'faucet', 'trash', 'garbage', 'flashlight', 'vacation', 'airplane',
    'dollars', 'cents', 'color', 'gray', 'math ', 'math?'
)

$TimeSensitive = @(
    'current', 'currently', 'today', 'latest', 'reigning', 'this year',
    'last season', 'record holder', 'prime minister', 'number one',
    'richest', 'recent', 'so far', 'to date', 'right now'
)

# Rule 3: only objects that are single-coloured by definition.
$ColourAllowlist = @(
    'postbox', 'london bus', 'london taxi', 'black cab', 'uk passport',
    'snooker cue ball', 'referee', 'yellow card', 'red card', 'zebra crossing',
    'polar bear', 'flamingo', 'royal mail'
)

$AnswerHedges = @(' or ', '/', 'either', 'approximately', 'about ', 'roughly', 'around ')

# --- Load --------------------------------------------------------------------

if (-not (Test-Path -LiteralPath $Path)) { throw "File not found: $Path" }

$raw = Get-Content -LiteralPath $Path -Raw -Encoding UTF8
try { $items = $raw | ConvertFrom-Json } catch { throw "Invalid JSON in ${Path}: $($_.Exception.Message)" }
if ($items -isnot [System.Array]) { throw "Root of $Path must be a JSON array." }

Write-Host "Validating $($items.Count) questions from $Path" -ForegroundColor Cyan

# --- Structural --------------------------------------------------------------

if ($Strict -and $items.Count -ne $ExpectedCount) {
    Add-Failure "Count is $($items.Count), expected $ExpectedCount."
}

$index = 0
foreach ($item in $items) {
    $index++
    $label = "[$index]"
    $props = $item.PSObject.Properties.Name

    if ($props -notcontains 'question') { Add-Failure "$label missing 'question'."; continue }
    if ($props -notcontains 'answer') { Add-Failure "$label missing 'answer'."; continue }

    if ($Strict) {
        $extra = $props | Where-Object { $_ -notin @('question', 'answer') }
        if ($extra) { Add-Failure "$label has disallowed fields: $($extra -join ', ')" }
    }

    $q = [string]$item.question
    $a = [string]$item.answer

    if ([string]::IsNullOrWhiteSpace($q)) { Add-Failure "$label question is empty."; continue }
    if ([string]::IsNullOrWhiteSpace($a)) { Add-Failure "$label answer is empty."; continue }

    $qLower = $q.ToLowerInvariant()
    $aLower = $a.ToLowerInvariant()
    $qNorm = Get-NormalisedText $q

    # Rule 12: ASCII only.
    foreach ($pair in @(@{ n = 'question'; v = $q }, @{ n = 'answer'; v = $a })) {
        $bad = [regex]::Matches($pair.v, '[^\x00-\x7F]')
        if ($bad.Count -gt 0) {
            Add-Failure "$label non-ASCII in $($pair.n): '$($pair.v)'"
        }
    }

    # Rule 5: question length.
    $wordCount = ($q -split '\s+' | Where-Object { $_ }).Count
    if ($wordCount -gt 12) { Add-Failure "$label question is $wordCount words (max 12): '$q'" }
    if ($q.Length -gt 80) { Add-Failure "$label question is $($q.Length) chars (max 80): '$q'" }
    if ($q -notmatch '\?$') { Add-Failure "$label question does not end with '?': '$q'" }

    # Rule 4: answer shape.
    $aWords = ($a -split '\s+' | Where-Object { $_ }).Count
    if ($aWords -gt 4) { Add-Failure "$label answer is $aWords words (max 4): '$a'" }
    foreach ($hedge in $AnswerHedges) {
        if ($aLower -like "*$hedge*") { Add-Failure "$label answer contains hedge '$($hedge.Trim())': '$a'" }
    }

    # Rule 1: bare superlatives.
    if ($SuperlativeAllowlist -notcontains $qNorm) {
        foreach ($sup in $BareSuperlatives) {
            if ($qNorm -match "\b$sup\b") {
                Add-Failure "$label ambiguous superlative '$sup': '$q'"
            }
        }
    }

    # Rule 2: "only X that Y" claims.
    if ($qLower -match '\bonly\b') {
        Add-Failure "$label uses an 'only ...' claim: '$q'"
    }

    # Rule 3: colour questions.
    if ($qLower -match 'what colou?r (is|are)') {
        $allowed = $false
        foreach ($subject in $ColourAllowlist) {
            if ($qLower -like "*$subject*") { $allowed = $true; break }
        }
        if (-not $allowed) { Add-Failure "$label colour question on a multi-coloured subject: '$q'" }
    }

    # Rule 7: UK English.
    foreach ($us in $Americanisms) {
        if ($qLower -like "*$us*" -or $aLower -like "*$us*") {
            Add-Failure "$label Americanism '$($us.Trim())': '$q' / '$a'"
        }
    }

    # Rule 8: time-sensitive.
    foreach ($ts in $TimeSensitive) {
        if ($qLower -like "*$ts*" -or $aLower -like "*$ts*") {
            Add-Failure "$label time-sensitive term '$ts': '$q' / '$a'"
        }
    }

    # Rule 6: spelled-out numbers in answers.
    if ($aLower -match '^(one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve)$') {
        Add-Failure "$label answer should use digits, not words: '$a'"
    }

    # No multiple choice.
    if ($q -match '\b[abcd]\)' -or $qLower -match '\bwhich of the following\b') {
        Add-Failure "$label looks like multiple choice: '$q'"
    }
}

# --- Arithmetic self-check ---------------------------------------------------

$mathsChecked = 0
foreach ($item in $items) {
    $q = [string]$item.question
    $m = [regex]::Match($q, '^\s*What is\s+(\d+)\s*([\+\-x\*])\s*(\d+)\s*\?\s*$', 'IgnoreCase')
    if (-not $m.Success) { continue }

    $left = [int]$m.Groups[1].Value
    $op = $m.Groups[2].Value.ToLowerInvariant()
    $right = [int]$m.Groups[3].Value

    switch ($op) {
        '+' { $expected = $left + $right }
        '-' { $expected = $left - $right }
        default { $expected = $left * $right }
    }

    $mathsChecked++
    $actual = ([string]$item.answer).Trim()
    if ($actual -ne [string]$expected) {
        Add-Failure "ARITHMETIC '$q' answer is '$actual', should be '$expected'."
    }

    # Rule: two-digit for +/-, single-digit for x.
    if ($op -eq 'x' -or $op -eq '*') {
        if ($left -gt 9 -or $right -gt 9) { Add-Failure "'$q' multiplication must use single digits." }
    }
    else {
        if ($left -gt 99 -or $right -gt 99) { Add-Failure "'$q' must use two-digit operands." }
        if ($expected -lt 0) { Add-Failure "'$q' gives a negative answer." }
    }
}

# --- Duplicates --------------------------------------------------------------

$seen = @{}
foreach ($item in $items) {
    $norm = Get-NormalisedText ([string]$item.question)
    if ($seen.ContainsKey($norm)) {
        Add-Failure "Duplicate question: '$($item.question)'"
    }
    else { $seen[$norm] = $true }
}

# Near-duplicates (Jaccard over word sets).
$tokenSets = @()
foreach ($item in $items) {
    $words = (Get-NormalisedText ([string]$item.question)) -split ' ' | Where-Object { $_ }
    $set = New-Object System.Collections.Generic.HashSet[string]
    foreach ($w in $words) { $set.Add($w) | Out-Null }
    $tokenSets += , $set
}
for ($i = 0; $i -lt $tokenSets.Count; $i++) {
    for ($j = $i + 1; $j -lt $tokenSets.Count; $j++) {
        $a = $tokenSets[$i]; $b = $tokenSets[$j]
        if ($a.Count -eq 0 -or $b.Count -eq 0) { continue }
        $inter = 0
        foreach ($w in $a) { if ($b.Contains($w)) { $inter++ } }
        $union = $a.Count + $b.Count - $inter
        if ($union -gt 0 -and (($inter / $union) -gt 0.8)) {
            Add-Warning "Near-duplicate: '$($items[$i].question)' ~ '$($items[$j].question)'"
        }
    }
}

# --- Rule 10: stem runs ------------------------------------------------------

$stems = @{}
foreach ($item in $items) {
    $words = (Get-NormalisedText ([string]$item.question)) -split ' ' | Where-Object { $_ }
    if ($words.Count -lt 4) { continue }
    $stem = ($words[0..3]) -join ' '
    if (-not $stems.ContainsKey($stem)) { $stems[$stem] = 0 }
    $stems[$stem]++
}
foreach ($stem in $stems.Keys) {
    if ($stems[$stem] -gt 6) {
        Add-Failure "Template run: $($stems[$stem]) questions start '$stem' (max 6)."
    }
}

# --- Rule 11: answer repetition ---------------------------------------------

$answerCounts = @{}
foreach ($item in $items) {
    $key = Get-NormalisedText ([string]$item.answer)
    if (-not $answerCounts.ContainsKey($key)) { $answerCounts[$key] = 0 }
    $answerCounts[$key]++
}
foreach ($key in $answerCounts.Keys) {
    # 'true'/'false' are inherent to the True or False format, so exempt.
    if ($key -in @('true', 'false')) { continue }
    if ($answerCounts[$key] -gt 8) {
        Add-Failure "Answer '$key' used $($answerCounts[$key]) times (max 8)."
    }
}

# --- Category quotas and spread (pool mode) ----------------------------------

$hasCategory = $items[0].PSObject.Properties.Name -contains 'category'
if ($hasCategory -and -not $Strict) {
    $quotas = @{
        maths = 20; animals = 50; science = 45; general = 45; geography = 40
        food  = 40; sports = 40; music = 40; truefalse = 40; body = 40
    }
    $main = $items | Where-Object { -not $_.reserve }
    $byCat = $main | Group-Object -Property category
    foreach ($cat in $quotas.Keys) {
        $found = ($byCat | Where-Object { $_.Name -eq $cat }).Count
        if ($null -eq $found) { $found = 0 }
        if ($found -ne $quotas[$cat]) {
            Add-Warning "Category '$cat' has $found questions, quota is $($quotas[$cat])."
        }
    }
    foreach ($g in $byCat) {
        if (-not $quotas.ContainsKey($g.Name)) { Add-Failure "Unknown category '$($g.Name)'." }
    }
}

# Any contiguous block of 20 should span at least 6 categories.
if ($hasCategory -and $items.Count -ge 20) {
    $broadcast = @($items | Where-Object { -not $_.reserve })
    for ($i = 0; $i + 20 -le $broadcast.Count; $i += 20) {
        $block = $broadcast[$i..($i + 19)]
        $distinct = ($block | Select-Object -ExpandProperty category -Unique).Count
        if ($distinct -lt 6) {
            Add-Warning "Day $([int]($i / 20) + 1) spans only $distinct categories (min 6)."
        }
    }
}

# --- Read-aloud timing -------------------------------------------------------

$totalWords = 0
foreach ($item in $items) {
    $totalWords += ($item.question -split '\s+' | Where-Object { $_ }).Count
    $totalWords += ($item.answer -split '\s+' | Where-Object { $_ }).Count
}
if ($items.Count -gt 0) {
    $secondsPerDay = ($totalWords / $items.Count) * 20 / 2.5
    Write-Host ("Estimated read time: {0:N0}s per 20-question day (words only, excl. banter)." -f $secondsPerDay) -ForegroundColor DarkGray
}

# --- Fact-check merge --------------------------------------------------------

if ($FactCheckPath) {
    if (-not (Test-Path -LiteralPath $FactCheckPath)) { throw "Fact-check file not found: $FactCheckPath" }
    $fc = Get-Content -LiteralPath $FactCheckPath -Raw -Encoding UTF8 | ConvertFrom-Json

    $byId = @{}
    foreach ($item in $items) { if ($item.id) { $byId[[string]$item.id] = $item } }

    foreach ($result in $fc) {
        $item = $byId[[string]$result.id]
        if (-not $item) { Add-Failure "FACTCHECK unknown id '$($result.id)'."; continue }

        $accepted = @()
        if ($item.acceptedAnswers) { $accepted = @($item.acceptedAnswers) }
        $accepted += [string]$item.answer
        $acceptedNorm = $accepted | ForEach-Object { Get-NormalisedText $_ }

        $blind = Get-NormalisedText ([string]$result.answer)
        $matched = $acceptedNorm -contains $blind
        if (-not $matched) {
            Add-Failure "FACTCHECK mismatch on '$($item.question)': ours '$($item.answer)', blind '$($result.answer)'."
        }
        if ([int]$result.difficulty -gt 2) {
            Add-Failure "FACTCHECK too hard (difficulty $($result.difficulty)): '$($item.question)'"
        }
        if ($result.ambiguous -eq $true) {
            # Reviewer landed on an answer we accept, so the noted alternative is a
            # nuance for the presenter rather than a trap. Genuine traps fail above.
            if ($matched) {
                Add-Warning "FACTCHECK nuance on '$($item.question)' - $($result.reason)"
            }
            else {
                Add-Failure "FACTCHECK ambiguous: '$($item.question)' - $($result.reason)"
            }
        }
    }

    $missing = $items | Where-Object { $_.id -and ($fc.id -notcontains $_.id) }
    if ($missing) { Add-Warning "$($missing.Count) questions have no fact-check result." }
}

# --- Report ------------------------------------------------------------------

Write-Host ''
Write-Host "Arithmetic questions verified: $mathsChecked" -ForegroundColor DarkGray

if ($script:Warnings.Count -gt 0) {
    Write-Host ''
    Write-Host "WARNINGS ($($script:Warnings.Count))" -ForegroundColor Yellow
    $script:Warnings | ForEach-Object { Write-Host "  - $_" -ForegroundColor Yellow }
}

if ($script:Failures.Count -gt 0) {
    Write-Host ''
    Write-Host "FAILURES ($($script:Failures.Count))" -ForegroundColor Red
    $script:Failures | ForEach-Object { Write-Host "  - $_" -ForegroundColor Red }
    Write-Host ''
    Write-Host 'FAILED' -ForegroundColor Red
    exit 1
}

Write-Host ''
Write-Host 'PASSED' -ForegroundColor Green
exit 0
