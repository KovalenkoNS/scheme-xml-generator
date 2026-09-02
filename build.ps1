$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
$env:CGO_ENABLED = '0'

go test ./...
if ($LASTEXITCODE -ne 0) {
    throw "Тесты завершились с кодом $LASTEXITCODE; сборка остановлена."
}
go build -trimpath -ldflags='-s -w' -o 'XmlSchemeGenerator.exe' ./cmd/schemegen
if ($LASTEXITCODE -ne 0) {
    throw "Компиляция завершилась с кодом $LASTEXITCODE."
}

Write-Host "Сборка завершена: $(Join-Path $PSScriptRoot 'XmlSchemeGenerator.exe')"
