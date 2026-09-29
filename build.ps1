# Builds the standalone generator only after SDD, Go and UI checks pass.
# Tests use isolated fixtures; the executable embeds the current two-page interface.
$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
$env:CGO_ENABLED = '0'

node scripts/sdd.cjs check
if ($LASTEXITCODE -ne 0) {
    throw "SDD/rules check failed with code $LASTEXITCODE; build stopped."
}
go test ./... -count=1
if ($LASTEXITCODE -ne 0) {
    throw "Go tests failed with code $LASTEXITCODE; build stopped."
}
node --test tests/sdd/sdd.test.cjs tests/web/workspace-model.test.cjs tests/preview/parser.test.cjs
if ($LASTEXITCODE -ne 0) {
    throw "UI checks failed with code $LASTEXITCODE; build stopped."
}
go build -trimpath -ldflags='-s -w' -o 'XmlSchemeGenerator.exe' ./cmd/schemegen
if ($LASTEXITCODE -ne 0) {
    throw "Compilation failed with code $LASTEXITCODE."
}

Write-Host "Build ready: $(Join-Path $PSScriptRoot 'XmlSchemeGenerator.exe')"
