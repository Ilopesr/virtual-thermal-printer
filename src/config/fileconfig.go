package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// FileConfig representa o arquivo de configuração virtual-printer.conf
type FileConfig struct {
	PrinterName string
	IPPPort     int
	WebPort     int
	PaperWidth  string
	OutputDir   string
	LogFile     string
	AutoOpen    bool   // abre browser ao iniciar
	SaveFormat  string // "all", "txt", "html", "raw"
}

var DefaultFileConfig = FileConfig{
	PrinterName: "Virtual-Thermal-Printer",
	IPPPort:     631,
	WebPort:     8080,
	PaperWidth:  "80",
	OutputDir:   "./jobs",
	LogFile:     "",
	AutoOpen:    false,
	SaveFormat:  "all",
}

// LoadConfig carrega um arquivo .conf estilo INI/TOML simples
// Formato: chave = valor (linhas com # são comentários)
func LoadConfig(path string) (*FileConfig, error) {
	cfg := DefaultFileConfig

	f, err := os.Open(path)
	if err != nil {
		return &cfg, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		// Remove comentários inline
		if idx := strings.Index(line, " #"); idx > 0 {
			line = strings.TrimSpace(line[:idx])
		}
		// Suporta [section] - ignora
		if strings.HasPrefix(line, "[") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// Remove aspas
		val = strings.Trim(val, `"'`)

		switch strings.ToLower(key) {
		case "printer_name", "printername", "name":
			cfg.PrinterName = val
		case "ipp_port", "ippport", "port":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.IPPPort = n
			}
		case "web_port", "webport":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.WebPort = n
			}
		case "paper_width", "paperwidth", "width":
			cfg.PaperWidth = val
		case "output_dir", "outputdir", "output":
			cfg.OutputDir = val
		case "log_file", "logfile":
			cfg.LogFile = val
		case "auto_open", "autoopen":
			cfg.AutoOpen = val == "true" || val == "1" || val == "yes"
		case "save_format", "saveformat":
			cfg.SaveFormat = val
		}
	}
	return &cfg, scanner.Err()
}

// SaveDefaultConfig cria um arquivo de configuração de exemplo
func SaveDefaultConfig(path string) error {
	content := `# Virtual Thermal Printer - Configuração
# Edite e reinicie o servidor para aplicar

[printer]
printer_name = "Virtual-Thermal-Printer"
paper_width  = "80"        # 58, 80 ou 110 (mm)

[server]
ipp_port = 631             # porta IPP (631 padrão; use >1024 sem sudo)
web_port = 8080            # interface web

[output]
output_dir  = "./jobs"     # diretório para salvar jobs
save_format = "all"        # all | txt | html | raw
log_file    = ""           # vazio = stdout

[ui]
auto_open = false          # abre browser automaticamente ao iniciar
`
	return os.WriteFile(path, []byte(content), 0644)
}

func (c *FileConfig) ToConfig(version string) *Config {
	return &Config{
		PrinterName: c.PrinterName,
		IPPPort:     c.IPPPort,
		WebPort:     c.WebPort,
		PaperWidth:  c.PaperWidth,
		OutputDir:   c.OutputDir,
		Version:     version,
		SaveFormat:  c.SaveFormat,
	}
}

// Validate verifica configurações
func (c *FileConfig) Validate() error {
	if c.PrinterName == "" {
		return fmt.Errorf("printer_name não pode ser vazio")
	}
	if c.IPPPort < 1 || c.IPPPort > 65535 {
		return fmt.Errorf("ipp_port inválido: %d", c.IPPPort)
	}
	if c.WebPort < 1 || c.WebPort > 65535 {
		return fmt.Errorf("web_port inválido: %d", c.WebPort)
	}
	switch c.PaperWidth {
	case "58", "80", "110":
	default:
		return fmt.Errorf("paper_width inválido: %s (use 58, 80 ou 110)", c.PaperWidth)
	}
	switch c.SaveFormat {
	case "all", "txt", "html", "raw":
	default:
		return fmt.Errorf("save_format inválido: %s", c.SaveFormat)
	}
	return nil
}
