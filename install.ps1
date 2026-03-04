$ErrorActionPreference = "Stop"

$repo = "Pankaj3112/labelr"
$installDir = "$env:LOCALAPPDATA\labelr"

# Detect architecture
$arch = if ([Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
} else {
    Write-Error "Unsupported architecture: 32-bit systems are not supported"
    exit 1
}

# Get latest release tag
$latestUrl = "https://github.com/$repo/releases/latest"
$response = Invoke-WebRequest -Uri $latestUrl -MaximumRedirection 0 -ErrorAction SilentlyContinue -UseBasicParsing
$redirectUrl = $response.Headers.Location
if (-not $redirectUrl) {
    $response = Invoke-WebRequest -Uri $latestUrl -UseBasicParsing
    $redirectUrl = $response.BaseResponse.ResponseUri.AbsoluteUri
}
$latest = ($redirectUrl -split "/")[-1]

if (-not $latest) {
    Write-Error "Failed to find latest release"
    exit 1
}

Write-Host "Installing labelr $latest (windows/$arch)..."

$filename = "labelr_windows_${arch}.zip"
$url = "https://github.com/$repo/releases/download/$latest/$filename"

$tmpDir = New-Item -ItemType Directory -Path (Join-Path $env:TEMP "labelr-install-$(Get-Random)")

try {
    $zipPath = Join-Path $tmpDir $filename
    Invoke-WebRequest -Uri $url -OutFile $zipPath -UseBasicParsing
    Expand-Archive -Path $zipPath -DestinationPath $tmpDir -Force

    if (-not (Test-Path $installDir)) {
        New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    }

    Copy-Item (Join-Path $tmpDir "labelr.exe") -Destination (Join-Path $installDir "labelr.exe") -Force

    # Add to PATH if not already present
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$installDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
        Write-Host "Added $installDir to your PATH (restart your terminal for it to take effect)"
    }
} finally {
    Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
}

Write-Host ""
Write-Host "labelr $latest installed to $installDir\labelr.exe"
Write-Host ""
Write-Host "Get started:"
Write-Host "  labelr setup    # first-time setup"
