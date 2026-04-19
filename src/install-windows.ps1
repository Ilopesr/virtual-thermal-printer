# install-windows.ps1 — Instala a impressora virtual no Windows
# Execute como Administrador: Right-click → "Run as Administrator"

param(
    [string]$PrinterName = "Virtual-Thermal-Printer",
    [int]$IPPPort = 631
)

$IPPUrl = "http://localhost:$IPPPort/printers/$PrinterName"
$PortName = "IP_VTP_$PrinterName"

Write-Host "============================================" -ForegroundColor Cyan
Write-Host "  Virtual Thermal Printer - Setup Windows" -ForegroundColor Cyan
Write-Host "============================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Impressora : $PrinterName"
Write-Host "URL IPP    : $IPPUrl"
Write-Host ""

# Verifica se está rodando como Admin
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "ERRO: Execute como Administrador!" -ForegroundColor Red
    exit 1
}

# Verifica servidor
Write-Host "Verificando servidor IPP..." -ForegroundColor Yellow
try {
    $resp = Invoke-WebRequest -Uri "http://localhost:$IPPPort/" -TimeoutSec 3 -UseBasicParsing
    Write-Host "Servidor IPP OK" -ForegroundColor Green
} catch {
    Write-Host "AVISO: Servidor não responde em localhost:$IPPPort" -ForegroundColor Yellow
    Write-Host "Inicie o servidor primeiro com: virtual-printer.exe"
}

# Remove instalação anterior
if (Get-Printer -Name $PrinterName -ErrorAction SilentlyContinue) {
    Write-Host "Removendo instalação anterior..." -ForegroundColor Yellow
    Remove-Printer -Name $PrinterName
}
if (Get-PrinterPort -Name $PortName -ErrorAction SilentlyContinue) {
    Remove-PrinterPort -Name $PortName
}

# Cria porta IPP
Write-Host "Criando porta IPP..." -ForegroundColor Yellow
Add-PrinterPort -Name $PortName -PrinterHostAddress "localhost" -PortNumber $IPPPort

# Adiciona impressora com driver Generic/Text
# O driver "MS Publisher Imagesetter" ou "Generic / Text Only" funciona bem para IPP
$driverName = "Generic / Text Only"
if (-not (Get-PrinterDriver -Name $driverName -ErrorAction SilentlyContinue)) {
    $driverName = "Microsoft XPS Document Writer"
}

Write-Host "Adicionando impressora ($driverName)..." -ForegroundColor Yellow
Add-Printer -Name $PrinterName `
            -DriverName $driverName `
            -PortName $PortName `
            -Comment "Virtual Thermal Printer - Emulador de Cupom" `
            -Location "Virtual"

Write-Host ""
Write-Host "Instalação concluída!" -ForegroundColor Green
Write-Host ""
Write-Host "A impressora '$PrinterName' está disponível." -ForegroundColor Cyan
Write-Host ""
Write-Host "Notas:" -ForegroundColor Yellow
Write-Host "  - Painel de controle > Dispositivos e Impressoras > $PrinterName"
Write-Host "  - Para melhor compatibilidade IPP, adicione manualmente:"
Write-Host "    Impressoras > Adicionar > Impressora de rede > Digitar URL: $IPPUrl"
Write-Host ""
Write-Host "Interface web: http://localhost:8080" -ForegroundColor Cyan
