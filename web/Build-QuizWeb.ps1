<#
.SYNOPSIS
    Builds the quiz web container image.

.DESCRIPTION
    The build context is the 99-quiz directory so that questions.json is in
    scope. go vet and go test run inside the builder stage, so a failing test
    means no image is produced.

.EXAMPLE
    .\Build-QuizWeb.ps1
    .\Build-QuizWeb.ps1 -Tag 99quiz:2026-09-02
#>
[CmdletBinding()]
param(
    [string]$Tag = '99quiz:latest'
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

Push-Location $root
try {
    # Docker writes build progress to stderr, which 'Stop' would treat as fatal.
    $ErrorActionPreference = 'Continue'
    docker build -f web/Dockerfile -t $Tag . 2>&1 | ForEach-Object { "$_" }
    if ($LASTEXITCODE -ne 0) {
        Write-Host 'BUILD FAILED' -ForegroundColor Red
        exit 1
    }
}
finally {
    Pop-Location
}

$size = docker image inspect $Tag --format '{{.Size}}'
Write-Host ("Built {0} ({1:N1} MB)" -f $Tag, ($size / 1MB)) -ForegroundColor Green
