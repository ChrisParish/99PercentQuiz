<#
.SYNOPSIS
    Runs the quiz web container.

.DESCRIPTION
    Serves on the given host port. SSL is handled upstream by the reverse proxy,
    so the container speaks plain HTTP.

.EXAMPLE
    .\Start-QuizWeb.ps1
    .\Start-QuizWeb.ps1 -Port 9090 -Title 'Breakfast 99% Quiz'
#>
[CmdletBinding()]
param(
    [string]$Tag = '99quiz:latest',
    [int]$Port = 8080,
    [string]$Title,
    [switch]$Detach
)

$ErrorActionPreference = 'Stop'

$dockerArgs = @('run', '--rm', '-p', "${Port}:8080", '--name', '99quiz')
if ($Detach) { $dockerArgs += '-d' }
if ($Title) { $dockerArgs += @('-e', "QUIZ_TITLE=$Title") }
$dockerArgs += $Tag

Write-Host "Starting on http://localhost:$Port" -ForegroundColor Cyan
docker @dockerArgs
