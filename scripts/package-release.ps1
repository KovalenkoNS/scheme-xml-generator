# Packages a previously verified Windows build for the GitHub-only Host catalogue.
# The allowlist contains only the executable and distribution guide, never local data or libraries.
param([string]$Executable = 'XmlSchemeGenerator.exe')
$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$executablePath = if ([IO.Path]::IsPathRooted($Executable)) { $Executable } else { Join-Path $repositoryRoot $Executable }
$executablePath = (Resolve-Path -LiteralPath $executablePath).Path
if ([IO.Path]::GetFileName($executablePath) -ne 'XmlSchemeGenerator.exe') { throw 'Expected XmlSchemeGenerator.exe' }
$releaseRoot = Join-Path $repositoryRoot 'dist'
[IO.Directory]::CreateDirectory($releaseRoot) | Out-Null
$archivePath = Join-Path $releaseRoot 'XmlSchemeGenerator-windows-amd64.zip'
if (Test-Path -LiteralPath $archivePath) { throw 'Release archive already exists; choose a new reviewed release instead of overwriting it.' }
Add-Type -AssemblyName System.IO.Compression.FileSystem
Add-Type -AssemblyName System.IO.Compression
$archive = [IO.Compression.ZipFile]::Open($archivePath, [IO.Compression.ZipArchiveMode]::Create)
try {
    [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive, $executablePath, 'XmlSchemeGenerator.exe') | Out-Null
    [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive, (Join-Path $repositoryRoot 'docs/distribution/README.md'), 'README.md') | Out-Null
} finally { $archive.Dispose() }
$hash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
[IO.File]::WriteAllText($archivePath + '.sha256', $hash + '  ' + [IO.Path]::GetFileName($archivePath) + "`n", (New-Object Text.UTF8Encoding($false)))
Write-Output $archivePath
Write-Output ('SHA256 ' + $hash)
