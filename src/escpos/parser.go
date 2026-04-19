package escpos

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Document representa o resultado do parse ESC/POS
type Document struct {
	Lines  []Line
	Width  int // colunas de texto
}

// Line é uma linha do cupom com formatação
type Line struct {
	Text      string
	Bold      bool
	Underline bool
	DoubleH   bool // altura dupla
	DoubleW   bool // largura dupla
	Align     int  // 0=esq, 1=centro, 2=dir
	Separator bool // linha divisória
	Feed      bool // linha em branco (paper feed)
	IsBarcode bool
	IsCut     bool
	FontB     bool // fonte menor
}

// Parse interpreta dados ESC/POS e retorna um Document
func Parse(data []byte, cols int) *Document {
	doc := &Document{Width: cols}
	if cols == 0 {
		cols = 48
		doc.Width = 48
	}

	state := defaultState()
	i := 0

	var lineBytes []byte

	flush := func() {
		text := cleanText(lineBytes)
		if text != "" || state.feed {
			line := Line{
				Text:      text,
				Bold:      state.bold,
				Underline: state.underline,
				DoubleH:   state.doubleH,
				DoubleW:   state.doubleW,
				Align:     state.align,
				Feed:      state.feed,
				FontB:     state.fontB,
			}
			doc.Lines = append(doc.Lines, line)
		}
		lineBytes = nil
		state.feed = false
	}

	for i < len(data) {
		b := data[i]

		switch b {
		case 0x0A: // LF - nova linha
			flush()
			i++

		case 0x0D: // CR - ignora (algumas impressoras usam CR+LF)
			i++

		case 0x1B: // ESC
			i++
			if i >= len(data) {
				break
			}
			cmd := data[i]
			i++
			switch cmd {
			case '@': // ESC @ - inicializa impressora
				state = defaultState()

			case 'E': // ESC E n - bold
				if i < len(data) {
					state.bold = data[i] != 0
					i++
				}

			case '-': // ESC - n - underline
				if i < len(data) {
					state.underline = data[i] != 0
					i++
				}

			case 'a': // ESC a n - alinhamento
				if i < len(data) {
					state.align = int(data[i] & 0x03)
					i++
				}

			case '!': // ESC ! n - modo de impressão
				if i < len(data) {
					n := data[i]
					state.bold = (n & 0x08) != 0
					state.doubleH = (n & 0x10) != 0
					state.doubleW = (n & 0x20) != 0
					state.underline = (n & 0x80) != 0
					i++
				}

			case 'M': // ESC M n - seleção de fonte
				if i < len(data) {
					state.fontB = data[i] == 1
					i++
				}

			case 'G': // ESC G n - bold (modo 2)
				if i < len(data) {
					state.bold = data[i] != 0
					i++
				}

			case 'd': // ESC d n - feed n linhas
				if i < len(data) {
					n := int(data[i])
					i++
					flush()
					for k := 0; k < n; k++ {
						doc.Lines = append(doc.Lines, Line{Feed: true})
					}
				}

			case 'p': // ESC p - abertura de gaveta (ignora)
				if i+1 < len(data) {
					i += 2
				}

			case 'V': // ESC V - corte de papel
				flush()
				doc.Lines = append(doc.Lines, Line{IsCut: true})
				if i < len(data) {
					i++
				}

			case 'Z', 'T', 'R', 'c', 'f', 't':
				// Comandos com 1 parâmetro - pula
				if i < len(data) {
					i++
				}

			case '*': // ESC * - modo bit-image (pula)
				if i+2 < len(data) {
					nL := int(data[i+1])
					nH := int(data[i+2])
					skip := 3 + (nL+nH*256)*3
					i += skip
					if i > len(data) {
						i = len(data)
					}
				}

			case '3', '4', '5', '6', '7':
				if i < len(data) {
					i++
				}

			case 'b':
				if i < len(data) {
					i++
				}

			default:
				// Comando desconhecido - pula 1 byte se parece parâmetro
			}

		case 0x1D: // GS
			i++
			if i >= len(data) {
				break
			}
			cmd := data[i]
			i++
			switch cmd {
			case 'B': // GS B n - bold (alternativo)
				if i < len(data) {
					state.bold = data[i] != 0
					i++
				}

			case '!': // GS ! n - tamanho do caractere
				if i < len(data) {
					n := data[i]
					state.doubleH = (n>>4) > 0
					state.doubleW = (n&0x0F) > 0
					i++
				}

			case 'a': // GS a - habilita status (ignora)
				if i < len(data) {
					i++
				}

			case 'V': // GS V - corte
				flush()
				doc.Lines = append(doc.Lines, Line{IsCut: true})
				if i < len(data) {
					i++
				}

			case 'h', 'H', 'w', 'f', 'r': // GS barcode params
				if i < len(data) {
					i++
				}

			case 'k': // GS k - barcode
				if i < len(data) {
					btype := data[i]
					i++
					var bdata []byte
					if btype <= 6 {
						// Formato antigo: terminado com NUL
						for i < len(data) && data[i] != 0 {
							bdata = append(bdata, data[i])
							i++
						}
						if i < len(data) {
							i++ // pula NUL
						}
					} else {
						// Formato novo: comprimento explícito
						if i < len(data) {
							n := int(data[i])
							i++
							if i+n <= len(data) {
								bdata = data[i : i+n]
								i += n
							}
						}
					}
					flush()
					doc.Lines = append(doc.Lines, Line{
						IsBarcode: true,
						Text:      string(bdata),
						Align:     state.align,
					})
				}

			case 'L', 'W': // GS L/W - margem/área (ignora, 4 bytes)
				i += 4
				if i > len(data) {
					i = len(data)
				}

			case 'x': // GS x - densidade (ignora)
				if i < len(data) {
					i++
				}

			default:
				// GS desconhecido
			}

		case 0x10: // DLE
			i++
			if i < len(data) {
				i++ // pula parâmetro
			}

		case 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
			0x08, 0x09, 0x0B, 0x0C, 0x0E, 0x0F,
			0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
			0x18, 0x19, 0x1A, 0x1C, 0x1E, 0x1F:
			// Outros bytes de controle - ignora
			i++

		default:
			// Caractere imprimível
			lineBytes = append(lineBytes, b)
			i++
		}
	}

	// Flush linha pendente
	flush()

	return doc
}

type parseState struct {
	bold      bool
	underline bool
	doubleH   bool
	doubleW   bool
	align     int
	feed      bool
	fontB     bool
}

func defaultState() parseState {
	return parseState{}
}

func cleanText(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	// Remove bytes de controle residuais e normaliza
	var sb strings.Builder
	for _, c := range b {
		if c >= 0x20 && c < 0x7F {
			sb.WriteByte(c)
		} else if c >= 0x80 {
			// Tenta preservar Latin-1 / CP850 como UTF-8
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// Render converte um Document em texto plano formatado para terminal/arquivo
func (doc *Document) RenderText() string {
	var sb strings.Builder
	w := doc.Width
	if w == 0 {
		w = 48
	}

	for _, line := range doc.Lines {
		if line.IsCut {
			sb.WriteString(strings.Repeat("-", w) + "\n")
			sb.WriteString(fmt.Sprintf("%s[CORTE]%s\n", strings.Repeat(" ", (w-7)/2), strings.Repeat(" ", (w-6)/2)))
			sb.WriteString(strings.Repeat("-", w) + "\n")
			continue
		}
		if line.Feed {
			sb.WriteString("\n")
			continue
		}
		if line.IsBarcode {
			bc := renderBarcode(line.Text, w)
			sb.WriteString(applyAlign(bc, w, 1) + "\n")
			continue
		}

		text := line.Text
		n := utf8.RuneCountInString(text)

		// Aplica largura dupla (metade das colunas disponíveis)
		maxW := w
		if line.DoubleW {
			maxW = w / 2
		}
		if n > maxW {
			text = string([]rune(text)[:maxW])
			n = maxW
		}

		// Alinhamento
		aligned := applyAlign(text, w, line.Align)

		// Bold: wrap com asteriscos nos extremos (visual)
		if line.Bold && !line.DoubleH {
			aligned = "**" + strings.TrimRight(aligned, " ") + "**"
		}

		sb.WriteString(aligned + "\n")
	}

	return sb.String()
}

// RenderHTML converte um Document em HTML estilizado de cupom
func (doc *Document) RenderHTML(paperWidth string) string {
	w := doc.Width
	if w == 0 {
		w = 48
	}

	fontPx := "12px"
	widthPx := "280px"
	switch paperWidth {
	case "58":
		fontPx = "11px"
		widthPx = "200px"
	case "110":
		fontPx = "13px"
		widthPx = "380px"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<div style="font-family:'Courier New',monospace;font-size:%s;background:#fff;color:#111;padding:16px 12px;width:%s;line-height:1.5;white-space:pre;">`, fontPx, widthPx))

	for _, line := range doc.Lines {
		if line.IsCut {
			sb.WriteString(fmt.Sprintf(`<div style="border-top:2px dashed #aaa;margin:8px 0;text-align:center;font-size:10px;color:#aaa;">✂ corte</div>`))
			continue
		}
		if line.Feed {
			sb.WriteString(`<div style="height:0.8em"></div>`)
			continue
		}
		if line.IsBarcode {
			bc := renderBarcodeHTML(line.Text)
			align := cssAlign(line.Align)
			sb.WriteString(fmt.Sprintf(`<div style="text-align:%s;margin:4px 0">%s</div>`, align, bc))
			continue
		}

		text := escapeHTML(line.Text)
		n := utf8.RuneCountInString(line.Text)
		maxW := w
		if line.DoubleW {
			maxW = w / 2
		}
		if n > maxW {
			runes := []rune(line.Text)
			text = escapeHTML(string(runes[:maxW]))
		}

		var style strings.Builder
		style.WriteString("display:block;")

		align := cssAlign(line.Align)
		style.WriteString(fmt.Sprintf("text-align:%s;", align))

		if line.Bold {
			style.WriteString("font-weight:700;")
		}
		if line.Underline {
			style.WriteString("text-decoration:underline;")
		}
		if line.DoubleH {
			style.WriteString("font-size:1.8em;line-height:1.2;")
		}
		if line.DoubleW {
			style.WriteString("letter-spacing:.18em;")
		}
		if line.FontB {
			style.WriteString("font-size:.85em;")
		}

		sb.WriteString(fmt.Sprintf(`<span style="%s">%s</span>`, style.String(), text))
		sb.WriteString("\n")
	}

	sb.WriteString("</div>")
	return sb.String()
}

func applyAlign(text string, w, align int) string {
	n := utf8.RuneCountInString(text)
	if n >= w {
		return text
	}
	switch align {
	case 1: // centro
		total := w - n
		left := total / 2
		return strings.Repeat(" ", left) + text + strings.Repeat(" ", total-left)
	case 2: // direita
		return strings.Repeat(" ", w-n) + text
	default: // esquerda
		return text + strings.Repeat(" ", w-n)
	}
}

func cssAlign(a int) string {
	switch a {
	case 1:
		return "center"
	case 2:
		return "right"
	default:
		return "left"
	}
}

func renderBarcode(data string, w int) string {
	// Representação textual de barcode
	line1 := fmt.Sprintf("[BARCODE: %s]", data)
	bars := ""
	for _, c := range data {
		v := int(c)
		for i := 0; i < (v%3)+1; i++ {
			bars += "|"
		}
		bars += " "
	}
	if utf8.RuneCountInString(bars) > w {
		bars = bars[:w]
	}
	return line1 + "\n" + bars
}

func renderBarcodeHTML(data string) string {
	var sb strings.Builder
	sb.WriteString(`<div style="display:inline-block;font-family:monospace">`)
	// Simula barras verticais
	sb.WriteString(`<div style="display:flex;gap:1px;height:40px;align-items:stretch;margin-bottom:2px">`)
	for _, c := range data {
		v := int(c)
		w := 1 + (v%3)
		black := (v % 2) == 0
		color := "#111"
		if !black {
			color = "#fff"
		}
		sb.WriteString(fmt.Sprintf(`<div style="width:%dpx;background:%s;border:0.5px solid #eee"></div>`, w, color))
	}
	sb.WriteString("</div>")
	sb.WriteString(fmt.Sprintf(`<div style="text-align:center;font-size:10px;letter-spacing:2px">%s</div>`, escapeHTML(data)))
	sb.WriteString("</div>")
	return sb.String()
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
