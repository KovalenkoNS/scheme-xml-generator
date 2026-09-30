# Compiles the distributable generator with its embedded interface.
# Acceptance uses the installed GitHub module through the running Host, not an isolated test process.
$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
$env:CGO_ENABLED = '0'

go build -trimpath -ldflags='-s -w -H windowsgui' -o 'XmlSchemeGenerator.exe' ./cmd/schemegen
if ($LASTEXITCODE -ne 0) {
    throw "Compilation failed with code $LASTEXITCODE."
}

Write-Host "Build created (acceptance pending): $(Join-Path $PSScriptRoot 'XmlSchemeGenerator.exe')"
