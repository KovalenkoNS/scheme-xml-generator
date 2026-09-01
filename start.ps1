$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot

$executable = Join-Path $PSScriptRoot 'XmlSchemeGenerator.exe'
if (-not (Test-Path -LiteralPath $executable)) {
    & (Join-Path $PSScriptRoot 'build.ps1')
}

& $executable

