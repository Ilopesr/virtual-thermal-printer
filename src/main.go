package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"virtual-printer/config"
	"virtual-printer/ipp"
	"virtual-printer/ui"
	"virtual-printer/ws"
)

var version = "1.1.0"

func main() {
	confFile := flag.String("config", "", "Arquivo de configuracao (ex: virtual-printer.conf)")
	port := flag.Int("port", 0, "Porta IPP (padrao: 631)")
	width := flag.String("width", "", "Largura do papel: 58, 80, 110")
	name := flag.String("name", "", "Nome da impressora")
	output := flag.String("output", "", "Diretorio para salvar os jobs")
	webPort := flag.Int("web", 0, "Porta da interface web")
	saveFormat := flag.String("format", "", "Formato de salvamento: all, txt, html, raw")
	genConfig := flag.Bool("gen-config", false, "Gera arquivo de configuracao de exemplo e sai")
	flag.Parse()

	if *genConfig {
		path := "virtual-printer.conf"
		if err := config.SaveDefaultConfig(path); err != nil {
			fmt.Fprintf(os.Stderr, "Erro ao criar config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Arquivo criado: %s\n", path)
		os.Exit(0)
	}

	fileCfg := config.DefaultFileConfig
	cfgPath := *confFile
	if cfgPath == "" {
		if _, err := os.Stat("virtual-printer.conf"); err == nil {
			cfgPath = "virtual-printer.conf"
		}
	}
	if cfgPath != "" {
		loaded, err := config.LoadConfig(cfgPath)
		if err != nil && *confFile != "" {
			log.Fatalf("Erro ao carregar config %s: %v", cfgPath, err)
		}
		if err == nil {
			fileCfg = *loaded
			log.Printf("Config carregado: %s", cfgPath)
		}
	}

	// Flags CLI sobrescrevem o arquivo
	if *port != 0 {
		fileCfg.IPPPort = *port
	}
	if *webPort != 0 {
		fileCfg.WebPort = *webPort
	}
	if *width != "" {
		fileCfg.PaperWidth = *width
	}
	if *name != "" {
		fileCfg.PrinterName = *name
	}
	if *output != "" {
		fileCfg.OutputDir = *output
	}
	if *saveFormat != "" {
		fileCfg.SaveFormat = *saveFormat
	}

	if err := fileCfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Configuracao invalida: %v\n", err)
		os.Exit(1)
	}

	cfg := fileCfg.ToConfig(version)

	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		log.Fatalf("Erro ao criar diretorio de jobs: %v", err)
	}

	if fileCfg.LogFile != "" {
		f, err := os.OpenFile(fileCfg.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("Erro ao abrir log file: %v", err)
		}
		defer f.Close()
		log.SetOutput(f)
	}

	fmt.Printf("\nVIRTUAL THERMAL PRINTER v%s\n", version)
	fmt.Printf("  Impressora : %s\n", cfg.PrinterName)
	fmt.Printf("  IPP        : ipp://localhost:%d/printers/%s\n", cfg.IPPPort, cfg.PrinterName)
	fmt.Printf("  Interface  : http://localhost:%d\n", cfg.WebPort)
	fmt.Printf("  Papel      : %smm\n", cfg.PaperWidth)
	fmt.Printf("  Jobs       : %s\n\n", cfg.OutputDir)

	hub := ws.NewHub()
	ippServer := ipp.NewServer(cfg, hub)
	webUI := ui.NewWebUI(cfg, ippServer, hub)

	go func() {
		if err := ippServer.Start(); err != nil {
			log.Fatalf("Erro IPP: %v", err)
		}
	}()
	go func() {
		if err := webUI.Start(); err != nil {
			log.Fatalf("Erro Web: %v", err)
		}
	}()

	fmt.Println("Linux  : sudo lpadmin -p " + cfg.PrinterName + " -E -v ipp://localhost:" + fmt.Sprintf("%d", cfg.IPPPort) + "/printers/" + cfg.PrinterName + " -m everywhere")
	fmt.Println("Windows: http://localhost:" + fmt.Sprintf("%d", cfg.IPPPort) + "/printers/" + cfg.PrinterName)
	fmt.Println("Web    : http://localhost:" + fmt.Sprintf("%d", cfg.WebPort))
	fmt.Println("\nCtrl+C para encerrar")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("\nEncerrando...")
}
