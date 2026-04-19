# Virtual Thermal Printer v1.1.0

Impressora virtual térmica com protocolo IPP/2.0 completo.
Compatível com CUPS (Linux/macOS) e Windows sem drivers adicionais.

## Binários prontos (pasta bin/)

| Arquivo                             | Sistema         |
|-------------------------------------|-----------------|
| virtual-printer-linux-amd64         | Linux 64-bit    |
| virtual-printer-linux-arm64         | Linux ARM (RPi) |
| virtual-printer-windows-amd64.exe   | Windows 64-bit  |

## Início rápido

### Linux
```bash
chmod +x bin/virtual-printer-linux-amd64

# Porta 631 requer root; ou use -port 6310 sem root
sudo ./bin/virtual-printer-linux-amd64 -port 6310

# Instalar no CUPS
sudo lpadmin -p Virtual-Thermal-Printer -E \
  -v ipp://localhost:631/printers/Virtual-Thermal-Printer \
  -m everywhere

# Testar
echo "Olá Mundo!" | lp -d Virtual-Thermal-Printer
lp -d Virtual-Thermal-Printer documento.pdf
```

### Windows
```
1. Execute bin/virtual-printer-windows-amd64.exe (como Administrador)
2. Painel de Controle > Dispositivos > Adicionar impressora
3. "A impressora não está na lista" > "Selecionar por nome"
4. URL: http://localhost:631/printers/Virtual-Thermal-Printer
5. Driver: Generic / Text Only
```

## Opções de linha de comando

```
-config   Arquivo .conf de configuração
-name     Nome da impressora       (padrão: Virtual-Thermal-Printer)
-port     Porta IPP                (padrão: 631)
-web      Porta da interface web   (padrão: 8080)
-width    Largura do papel mm      (padrão: 80) [58|80|110]
-output   Diretório de jobs        (padrão: ./jobs)
-format   Salvar: all|txt|html|raw (padrão: all)
-gen-config  Gera arquivo .conf de exemplo
```

## Arquivo de configuração

```bash
./virtual-printer -gen-config    # gera virtual-printer.conf
./virtual-printer                # usa automaticamente se existir
```

## Interface Web

Acesse http://localhost:8080 para:
- Ver jobs em **tempo real** via WebSocket
- Visualizar cupom formatado (texto e HTML)
- Ver comandos de instalação para Linux/Windows
- Limpar fila de jobs

## Arquitetura

```
Aplicativo → CUPS / Windows IPP → TCP:631 → virtual-printer
                                               ├── parser ESC/POS
                                               ├── renderiza cupom .txt + .html
                                               ├── salva em ./jobs/
                                               └── push WebSocket → http://localhost:8080
```

## Protocolos suportados

- **IPP/2.0** (RFC 8010 + RFC 8011) — Print-Job, Get-Printer-Attrs, Get-Jobs, etc.
- **ESC/POS** — comandos Epson: bold, align, double-width/height, barcode, cut
- **Formatos**: text/plain, application/pdf, application/postscript, image/jpeg, image/png, application/octet-stream

## Compilar do fonte (requer Go 1.18+)

```bash
cd src/
make all       # compila Linux + Windows
make run       # roda localmente
make run-58    # papel 58mm
make run-110   # papel 110mm
```

## Jobs salvos

Cada job gera até 3 arquivos em `./jobs/`:

| Arquivo                      | Conteúdo                        |
|------------------------------|---------------------------------|
| `job_001_NomeDoJob.bin`      | dados brutos recebidos          |
| `job_001_receipt.txt`        | cupom formatado em texto        |
| `job_001_receipt.html`       | visualização HTML com impressora|
