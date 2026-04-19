package render

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"virtual-printer/config"
	"virtual-printer/escpos"
)

type Renderer struct {
	cfg *config.Config
}

func NewRenderer(cfg *config.Config) *Renderer {
	return &Renderer{cfg: cfg}
}

func (r *Renderer) PaperCols() int {
	switch r.cfg.PaperWidth {
	case "58":
		return 32
	case "110":
		return 56
	default:
		return 48
	}
}

func (r *Renderer) Render(job *config.Job) {
	if len(job.Data) == 0 {
		return
	}
	cols := r.PaperCols()
	format := r.cfg.SaveFormat
	if format == "" {
		format = "all"
	}

	txtContent := r.BuildTXT(job, cols)
	if format == "all" || format == "txt" {
		txtPath := filepath.Join(r.cfg.OutputDir, fmt.Sprintf("job_%03d_receipt.txt", job.ID))
		os.WriteFile(txtPath, []byte(txtContent), 0644)
	}
	if format == "all" || format == "html" {
		htmlContent := r.BuildHTML(job, cols)
		htmlPath := filepath.Join(r.cfg.OutputDir, fmt.Sprintf("job_%03d_receipt.html", job.ID))
		os.WriteFile(htmlPath, []byte(htmlContent), 0644)
	}
}

func (r *Renderer) BuildTXT(job *config.Job, cols int) string {
	var sb strings.Builder
	w := cols
	sb.WriteString(divider(w, "=") + "\n")
	sb.WriteString(centerStr("VIRTUAL THERMAL PRINTER", w) + "\n")
	sb.WriteString(centerStr(r.cfg.PrinterName, w) + "\n")
	sb.WriteString(divider(w, "=") + "\n")
	sb.WriteString(fmt.Sprintf("Job     : #%d\n", job.ID))
	sb.WriteString(fmt.Sprintf("Nome    : %s\n", job.Name))
	if job.User != "" {
		sb.WriteString(fmt.Sprintf("Usuario : %s\n", job.User))
	}
	sb.WriteString(fmt.Sprintf("Formato : %s\n", job.Format))
	sb.WriteString(fmt.Sprintf("Tamanho : %s\n", HumanSize(job.Size)))
	sb.WriteString(fmt.Sprintf("Data    : %s\n", time.Now().Format("02/01/2006 15:04:05")))
	sb.WriteString(divider(w, "-") + "\n")
	sb.WriteString(r.extractBody(job, cols))
	sb.WriteString("\n" + divider(w, "=") + "\n")
	sb.WriteString(centerStr(fmt.Sprintf("FIM DO JOB #%d", job.ID), w) + "\n")
	sb.WriteString(divider(w, "=") + "\n\n\n")
	return sb.String()
}

func (r *Renderer) extractBody(job *config.Job, cols int) string {
	data := job.Data
	format := job.Format
	if isLikelyEscPos(data) || strings.Contains(format, "text") || strings.Contains(format, "octet") {
		doc := escpos.Parse(data, cols)
		rendered := doc.RenderText()
		if strings.TrimSpace(rendered) != "" {
			return rendered
		}
	}
	if isPrintable(data) {
		return wrapText(string(data), cols)
	}
	var sb strings.Builder
	sb.WriteString(centerStr("[Dados binarios - "+HumanSize(len(data))+"]", cols) + "\n")
	sb.WriteString(divider(cols, "-") + "\n")
	sb.WriteString(hexDump(data, cols))
	return sb.String()
}

func (r *Renderer) BuildHTML(job *config.Job, cols int) string {
	var body string
	data := job.Data
	if isLikelyEscPos(data) || strings.Contains(job.Format, "text") || strings.Contains(job.Format, "octet") {
		doc := escpos.Parse(data, cols)
		body = doc.RenderHTML(r.cfg.PaperWidth)
	} else if isPrintable(data) {
		escaped := escapeHTML(string(data))
		body = fmt.Sprintf(`<div style="font-family:'Courier New',monospace;font-size:12px;white-space:pre-wrap;padding:12px">%s</div>`, escaped)
	} else {
		body = fmt.Sprintf(`<div style="font-family:monospace;font-size:11px;white-space:pre;padding:12px">%s</div>`, escapeHTML(hexDump(data, cols)))
	}

	widthMap := map[string]string{"58": "220px", "80": "300px", "110": "400px"}
	widthPx := widthMap[r.cfg.PaperWidth]
	if widthPx == "" {
		widthPx = "300px"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="pt-br">
<head><meta charset="UTF-8"><title>Job #%d - %s</title>
<style>
body{font-family:sans-serif;background:#e0e0e0;display:flex;flex-direction:column;align-items:center;padding:24px}
.printer{background:#2a2a2a;padding:14px 16px 6px;border-radius:10px 10px 4px 4px;width:%s}
.slot{background:#111;height:5px;border-radius:2px}
.paper{background:#fff;box-shadow:0 6px 20px rgba(0,0,0,.25);min-height:80px;border-radius:0 0 4px 4px;width:%s;overflow:hidden}
.led{width:8px;height:8px;border-radius:50%%;background:#4caf50;display:inline-block;margin-right:8px}
.hb{display:flex;align-items:center;margin-bottom:8px}
.pn{font-size:10px;color:#aaa;font-family:monospace}
h2{color:#333;margin-bottom:4px;font-size:16px}.meta{font-size:12px;color:#777;margin-bottom:16px}
</style></head>
<body>
<h2>Virtual Thermal Printer</h2>
<div class="meta">Job #%d &bull; %s &bull; %s &bull; %s</div>
<div class="printer"><div class="hb"><span class="led"></span><span class="pn">%s &mdash; %smm</span></div><div class="slot"></div></div>
<div class="paper">%s</div>
</body></html>`,
		job.ID, job.Name, widthPx, widthPx,
		job.ID, job.Name, job.ReceivedAt, HumanSize(job.Size),
		r.cfg.PrinterName, r.cfg.PaperWidth, body)
}

func (r *Renderer) GetReceiptText(job *config.Job) string {
	txtPath := filepath.Join(r.cfg.OutputDir, fmt.Sprintf("job_%03d_receipt.txt", job.ID))
	if data, err := os.ReadFile(txtPath); err == nil {
		return string(data)
	}
	return r.BuildTXT(job, r.PaperCols())
}

func (r *Renderer) GetReceiptHTML(jobID int) ([]byte, error) {
	return os.ReadFile(filepath.Join(r.cfg.OutputDir, fmt.Sprintf("job_%03d_receipt.html", jobID)))
}

func (r *Renderer) HasHTML(jobID int) bool {
	_, err := os.Stat(filepath.Join(r.cfg.OutputDir, fmt.Sprintf("job_%03d_receipt.html", jobID)))
	return err == nil
}

func isLikelyEscPos(data []byte) bool {
	for _, b := range data {
		if b == 0x1B || b == 0x1D {
			return true
		}
	}
	return false
}

func isPrintable(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	p := 0
	for _, b := range data {
		if b >= 0x20 && b < 0x7F || b == '\n' || b == '\r' || b == '\t' {
			p++
		}
	}
	return float64(p)/float64(len(data)) > 0.80
}

func wrapText(s string, w int) string {
	var sb strings.Builder
	for _, line := range strings.Split(s, "\n") {
		runes := []rune(line)
		for len(runes) > 0 {
			end := w
			if end > len(runes) {
				end = len(runes)
			}
			sb.WriteString(string(runes[:end]) + "\n")
			runes = runes[end:]
		}
	}
	return sb.String()
}

func hexDump(data []byte, w int) string {
	var sb strings.Builder
	limit := 512
	if len(data) < limit {
		limit = len(data)
	}
	for i := 0; i < limit; i += 16 {
		end := i + 16
		if end > limit {
			end = limit
		}
		chunk := data[i:end]
		hex := ""
		for _, b := range chunk {
			hex += fmt.Sprintf("%02X ", b)
		}
		ascii := ""
		for _, b := range chunk {
			if b >= 0x20 && b < 0x7F {
				ascii += string(rune(b))
			} else {
				ascii += "."
			}
		}
		line := fmt.Sprintf("%04X: %-48s %s", i, hex, ascii)
		if utf8.RuneCountInString(line) > w {
			line = string([]rune(line)[:w])
		}
		sb.WriteString(line + "\n")
	}
	if len(data) > limit {
		sb.WriteString(fmt.Sprintf("... +%d bytes\n", len(data)-limit))
	}
	return sb.String()
}

func divider(w int, char string) string {
	return strings.Repeat(char, w)
}

func centerStr(s string, w int) string {
	n := utf8.RuneCountInString(s)
	if n >= w {
		return s
	}
	total := w - n
	left := total / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", total-left)
}

func HumanSize(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	} else if n < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/1024/1024)
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
