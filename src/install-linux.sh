#!/bin/bash
set -e

PRINTER_NAME="${1:-Virtual-Thermal-Printer}"
IPP_PORT="${2:-6310}"   # 🔥 melhor default
IPP_URL="ipp://localhost:${IPP_PORT}/printers/${PRINTER_NAME}"

echo "=============================================="
echo "  Virtual Thermal Printer - Instalação CUPS"
echo "=============================================="
echo ""
echo "Impressora : $PRINTER_NAME"
echo "URL IPP    : $IPP_URL"
echo ""

# 🔧 Garante que CUPS está rodando
echo "Verificando CUPS..."
if ! systemctl is-active --quiet cups; then
  echo "Iniciando CUPS..."
  sudo systemctl start cups
fi
echo "CUPS OK"

# 🔎 Verifica IPP de verdade (porta aberta)
echo "Verificando servidor IPP..."
if ! nc -z localhost "$IPP_PORT"; then
  echo "ERRO: Servidor IPP não está escutando em localhost:${IPP_PORT}"
  echo "Inicie com: ./virtual-printer -port ${IPP_PORT}"
  exit 1
fi
echo "Servidor IPP OK"

# 🧹 Remove impressora antiga
if lpstat -p "$PRINTER_NAME" >/dev/null 2>&1; then
  echo "Removendo instalação anterior..."
  sudo lpadmin -x "$PRINTER_NAME" || true
fi

# ➕ Adiciona impressora
echo "Adicionando impressora ao CUPS..."
if ! sudo lpadmin -p "$PRINTER_NAME" \
  -E \
  -v "$IPP_URL" \
  -m everywhere \
  -D "Virtual Thermal Printer (${PRINTER_NAME})" \
  -L "Virtual / Emulador"; then

  echo ""
  echo "⚠️ Falha com IPP Everywhere."
  echo "Tentando fallback (sem driver)..."

  sudo lpadmin -p "$PRINTER_NAME" -E -v "$IPP_URL"
fi

# 🎯 Define padrão
read -p "Definir como impressora padrão? [s/N] " def
if [[ "$def" =~ ^[sS]$ ]]; then
  sudo lpoptions -d "$PRINTER_NAME"
  echo "Definida como padrão."
fi

echo ""
echo "Instalação concluída!"
echo ""
echo "Testes:"
echo "  echo 'Teste' | lp -d $PRINTER_NAME"
echo "  lp -d $PRINTER_NAME arquivo.pdf"
echo ""
echo "Status:"
echo "  lpstat -p $PRINTER_NAME"
echo "  lpstat -o"