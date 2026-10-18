<#
.SYNOPSIS
    Runs the quiz web container.

.DESCRIPTION
    Serves on the given host port. SSL is handled upstream by the reverse proxy,
    so the container speaks plain HTTP.

    Swap state (which question is currently shown in each day's slots) is
    persisted to StateDir so it survives a restart. It's bind-mounted from the
    host and made world-writable, since the container runs as a non-root,
    unnamed UID (65532) and scratch images have no chown step to grant it
    ownership directly.

.EXAMPLE
    .\Start-QuizWeb.ps1
    .\Start-QuizWeb.ps1 -Port 9090 -Title 'Breakfast 99% Quiz'
    .\Start-QuizWeb.ps1 -StateDir C:\quiz-state
#>
[CmdletBinding()]
param(
    [string]$Tag = '99quiz:latest',
    [int]$Port = 8080,
    [string]$Title,
    [string]$StateDir = (Join-Path (Get-Location) 'quiz-state'),
    [switch]$Detach
)

$ErrorActionPreference = 'Stop'

New-Item -ItemType Directory -Force -Path $StateDir | Out-Null

$dockerArgs = @('run', '--rm', '-p', "${Port}:8080", '--name', '99quiz', '-v', "${StateDir}:/data/state")
if ($Detach) { $dockerArgs += '-d' }
if ($Title) { $dockerArgs += @('-e', "QUIZ_TITLE=$Title") }
$dockerArgs += $Tag

Write-Host "Starting on http://localhost:$Port (state in $StateDir)" -ForegroundColor Cyan
docker @dockerArgs

