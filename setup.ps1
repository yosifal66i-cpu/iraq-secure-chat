# IraqSecureChat - Auto Installer
# One-click setup: Run this script as Administrator

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$repoUrl = "https://github.com/yosifal66i-cpu/iraq-secure-chat"
$installDir = "$env:USERPROFILE\Desktop\IraqSecureChat"
$logFile = "$env:TEMP\iraqchat-install.log"

Write-Host "╔══════════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║       IraqSecureChat - Auto Installer        ║" -ForegroundColor Cyan
Write-Host "║    منصة المراسلة الآمنة للجهات الحكومية       ║" -ForegroundColor Cyan
Write-Host "╚══════════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""

# Step 1: Download code
Write-Host "[1/5] Downloading IraqSecureChat..." -ForegroundColor Yellow
if (Test-Path $installDir) { Remove-Item -Recurse -Force $installDir }
$zipUrl = "$repoUrl/archive/refs/heads/main.zip"
$zipPath = "$env:TEMP\iraqchat.zip"
Invoke-WebRequest -Uri $zipUrl -OutFile $zipPath
Expand-Archive -Path $zipPath -DestinationPath $installDir -Force
Remove-Item $zipPath -Force
Write-Host "  ✓ Downloaded to Desktop" -ForegroundColor Green

# Step 2: Check Docker
Write-Host "[2/5] Checking Docker..." -ForegroundColor Yellow
$dockerInstalled = $false
try { docker --version 2>$null; $dockerInstalled = $true } catch {}

if ($dockerInstalled) {
    Write-Host "  ✓ Docker found" -ForegroundColor Green
    Write-Host "[3/5] Starting services with Docker..." -ForegroundColor Yellow
    Set-Location "$installDir\iraq-secure-chat-main"
    docker-compose -f infra/docker-compose.yml up -d 2>&1 | Out-Null
    Write-Host "  ✓ All services started!" -ForegroundColor Green
} else {
    Write-Host "  ⚠ Docker not found. Installing minimal setup..." -ForegroundColor Yellow
    Write-Host "[3/5] Starting web client only..." -ForegroundColor Yellow
    # Check Node.js
    try { node --version 2>$null } catch {
        Write-Host "  ✗ Node.js required. Download from: https://nodejs.org" -ForegroundColor Red
        Start-Process "https://nodejs.org"
        exit 1
    }
    Set-Location "$installDir\iraq-secure-chat-main\web"
    npm install 2>&1 | Out-Null
    Write-Host "  ✓ Web client ready" -ForegroundColor Green
}

# Step 4: Create shortcuts
Write-Host "[4/5] Creating shortcuts..." -ForegroundColor Yellow
$shortcutPath = "$env:USERPROFILE\Desktop\IraqSecureChat.url"
@"
[InternetShortcut]
URL=http://localhost:3000
"@ | Out-File -FilePath $shortcutPath -Encoding ASCII

$desktopPath = [Environment]::GetFolderPath("Desktop")
$shortcutDir = "$desktopPath\IraqSecureChat"
New-Item -ItemType Directory -Path $shortcutDir -Force | Out-Null

# Step 5: Open browser
Write-Host "[5/5] Opening application..." -ForegroundColor Yellow
Start-Process "http://localhost:3000"

Write-Host ""
Write-Host "╔══════════════════════════════════════════════╗" -ForegroundColor Green
Write-Host "║     IraqSecureChat جاهز للاستخدام!           ║" -ForegroundColor Green
Write-Host "║                                              ║" -ForegroundColor Green
Write-Host "║  الموقع: http://localhost:3000               ║" -ForegroundColor Green
Write-Host "║  API:    http://localhost:8090               ║" -ForegroundColor Green
Write-Host "║                                              ║" -ForegroundColor Green
Write-Host "║  المسار: $installDir           ║" -ForegroundColor Green
Write-Host "╚══════════════════════════════════════════════╝" -ForegroundColor Green
Write-Host ""
Write-Host "اضغط أي مفتاح للخروج..."
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
