$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
$env:CGO_ENABLED = '0'

go test ./...
go build -trimpath -ldflags='-s -w' -o 'XmlSchemeGenerator.exe' ./cmd/schemegen

Write-Host "Сборка завершена: $(Join-Path $PSScriptRoot 'XmlSchemeGenerator.exe')"
