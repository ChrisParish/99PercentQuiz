<#
.SYNOPSIS
    Runs gofmt, go vet and the Go test suite in a container.

.DESCRIPTION
    Go is not installed locally, so all toolchain work happens inside
    golang:1.23-alpine. A named volume caches the Go build cache between runs.

.EXAMPLE
    .\Test-QuizWeb.ps1
    .\Test-QuizWeb.ps1 -Detailed
#>
[CmdletBinding()]
param(
    [switch]$Detailed
)

$ErrorActionPreference = 'Stop'

$web = $PSScriptRoot
$root = Split-Path -Parent $web

# go:embed cannot reach outside the package directory, so sync the pool in.
# pool.json rather than questions.json because the rotation needs categories.
New-Item -ItemType Directory -Force -Path (Join-Path $web 'data') | Out-Null
Copy-Item (Join-Path $root 'working\pool.json') (Join-Path $web 'data\pool.json') -Force

$fmt = docker run --rm -v "${web}:/src" -w /src golang:1.23-alpine gofmt -l .
if ($fmt) {
    Write-Host 'gofmt required on:' -ForegroundColor Red
    $fmt | ForEach-Object { Write-Host "  $_" -ForegroundColor Red }
    exit 1
}

$cmd = if ($Detailed) { 'go vet ./... && go test ./... -v' } else { 'go vet ./... && go test ./...' }

docker run --rm `
    -v "${web}:/src" `
    -v 99quiz-gocache:/root/.cache/go-build `
    -w /src golang:1.23-alpine sh -c $cmd

if ($LASTEXITCODE -ne 0) {
    Write-Host 'FAILED' -ForegroundColor Red
    exit 1
}

Write-Host 'PASSED' -ForegroundColor Green
