#!/bin/bash
# install-linux.sh — Instala a impressora virtual no CUPS

set -e

PRINTER_NAME="${1:-Virtual-Thermal-Printer}"
IPP_PORT="${2:-631}"
IPP_URL="ipp://localhost:${IPP_PORT}/printers/${PRINTER_NAME}"

echo "=============================================="
echo "  Virtual Thermal Printer - Instalação CUPS"
echo "=============================================="
echo ""
echo "Impressora : $PRINTER_NAME"
echo "URL IPP    : $IPP_URL"
echo ""

# Verifica se o servidor está rodando
echo "Verificando servidor IPP..."
if ! curl -s "http://localhost:${IPP_PORT}/" > /dev/null 2>&1; then
  echo "AVISO: Servidor IPP não responde em localhost:${IPP_PORT}"
  echo "Inicie o servidor primeiro com: ./virtual-printer"
  exit 1
fi
echo "Servidor IPP OK"

# Remove impressora antiga se existir
if lpstat -p "$PRINTER_NAME" 2>/dev/null; then
  echo "Removendo instalação anterior..."
  sudo lpadmin -x "$PRINTER_NAME" 2>/dev/null || true
fi

# Adiciona a impressora via IPP Everywhere (driverless)
echo "Adicionando impressora ao CUPS..."
sudo lpadmin -p "$PRINTER_NAME" \
  -E \
  -v "$IPP_URL" \
  -m everywhere \
  -D "Virtual Thermal Printer (${PRINTER_NAME})" \
  -L "Virtual / Emulador"

# Define como padrão (opcional)
read -p "Definir como impressora padrão? [s/N] " def
if [[ "$def" =~ ^[sS]$ ]]; then
  sudo lpoptions -d "$PRINTER_NAME"
  echo "Definida como padrão."
fi

echo ""
echo "Instalação concluída!"
echo ""
echo "Testar impressão:"
echo "  echo 'Teste de impressão' | lp -d $PRINTER_NAME"
echo "  lp -d $PRINTER_NAME arquivo.pdf"
echo "  lpr -P $PRINTER_NAME arquivo.txt"
echo ""
echo "Ver status:"
echo "  lpstat -p $PRINTER_NAME"
echo "  lpstat -o"
