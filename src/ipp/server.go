package ipp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"virtual-printer/config"
	"virtual-printer/render"
	"virtual-printer/ws"
)

const (
	IPP_VERSION_MAJOR = 0x02
	IPP_VERSION_MINOR = 0x00

	OP_PRINT_JOB         = 0x0002
	OP_VALIDATE_JOB      = 0x0004
	OP_CREATE_JOB        = 0x0005
	OP_SEND_DOCUMENT     = 0x0006
	OP_CANCEL_JOB        = 0x0008
	OP_GET_JOB_ATTRS     = 0x0009
	OP_GET_JOBS          = 0x000A
	OP_GET_PRINTER_ATTRS = 0x000B
	OP_IDENTIFY_PRINTER  = 0x003C

	STATUS_OK                   = 0x0000
	STATUS_CLIENT_ERROR_BAD_REQ = 0x0400
	STATUS_SERVER_ERROR         = 0x0500

	TAG_OPERATION_GROUP  = 0x01
	TAG_JOB_GROUP        = 0x02
	TAG_END_ATTRS        = 0x03
	TAG_PRINTER_GROUP    = 0x04

	TAG_INTEGER           = 0x21
	TAG_BOOLEAN           = 0x22
	TAG_ENUM              = 0x23
	TAG_TEXT_WITHOUT_LANG = 0x41
	TAG_NAME_WITHOUT_LANG = 0x42
	TAG_KEYWORD           = 0x44
	TAG_URI               = 0x45
	TAG_CHARSET           = 0x47
	TAG_NATURAL_LANG      = 0x48
	TAG_MIME_TYPE         = 0x49
	TAG_RESOLUTION        = 0x32
	TAG_RANGE_OF_INT      = 0x33
)

type Server struct {
	cfg      *config.Config
	Jobs     *config.JobStore
	renderer *render.Renderer
	Hub      *ws.Hub
}

func NewServer(cfg *config.Config, hub *ws.Hub) *Server {
	return &Server{
		cfg:      cfg,
		Jobs:     config.NewJobStore(),
		renderer: render.NewRenderer(cfg),
		Hub:      hub,
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIPP)
	mux.HandleFunc("/printers/", s.handleIPP)
	mux.HandleFunc("/ipp/print", s.handleIPP)

	addr := fmt.Sprintf(":%d", s.cfg.IPPPort)
	log.Printf("Servidor IPP escutando em %s", addr)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleIPP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Virtual Thermal Printer IPP/2.0 - %s\n", s.cfg.PrinterName)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20)) // max 32MB
	if err != nil || len(body) < 8 {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	req, docData, err := parseIPPRequest(body)
	if err != nil {
		http.Error(w, "Bad IPP", http.StatusBadRequest)
		return
	}

	log.Printf("[IPP] OP=0x%04X id=%d user=%q name=%q fmt=%q size=%d",
		req.Operation, req.RequestID, req.UserName, req.JobName, req.DocFormat, len(docData))

	var resp []byte
	switch req.Operation {
	case OP_PRINT_JOB:
		resp = s.handlePrintJob(req, docData)
	case OP_VALIDATE_JOB:
		resp = s.buildSimpleResponse(STATUS_OK, req.RequestID, 0)
	case OP_GET_PRINTER_ATTRS:
		resp = s.handleGetPrinterAttrs(req)
	case OP_GET_JOBS:
		resp = s.handleGetJobs(req)
	case OP_GET_JOB_ATTRS:
		resp = s.handleGetJobAttrs(req)
	case OP_CANCEL_JOB:
		resp = s.buildSimpleResponse(STATUS_OK, req.RequestID, 0)
	case OP_CREATE_JOB:
		resp = s.handleCreateJob(req)
	case OP_SEND_DOCUMENT:
		resp = s.handleSendDocument(req, docData)
	case OP_IDENTIFY_PRINTER:
		resp = s.buildSimpleResponse(STATUS_OK, req.RequestID, 0)
	default:
		log.Printf("[IPP] Operacao nao suportada: 0x%04X", req.Operation)
		resp = s.buildSimpleResponse(STATUS_CLIENT_ERROR_BAD_REQ, req.RequestID, 0)
	}

	w.Header().Set("Content-Type", "application/ipp")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(resp)))
	w.WriteHeader(http.StatusOK)
	w.Write(resp)
}

type IPPRequest struct {
	VersionMajor byte
	VersionMinor byte
	Operation    uint16
	RequestID    uint32
	Attrs        map[string]string
	UserName     string
	JobName      string
	DocFormat    string
	PrinterURI   string
	JobID        int
}

func parseIPPRequest(data []byte) (*IPPRequest, []byte, error) {
	r := &IPPRequest{
		VersionMajor: data[0],
		VersionMinor: data[1],
		Operation:    binary.BigEndian.Uint16(data[2:4]),
		RequestID:    binary.BigEndian.Uint32(data[4:8]),
		Attrs:        make(map[string]string),
		DocFormat:    "application/octet-stream",
	}

	pos := 8
	for pos < len(data) {
		tag := data[pos]
		pos++
		if tag == TAG_END_ATTRS {
			break
		}
		if tag <= 0x0F {
			continue
		}
		if pos+2 > len(data) {
			break
		}
		nameLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		pos += 2
		if pos+nameLen > len(data) {
			break
		}
		name := string(data[pos : pos+nameLen])
		pos += nameLen
		if pos+2 > len(data) {
			break
		}
		valLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		pos += 2
		if pos+valLen > len(data) {
			break
		}
		val := data[pos : pos+valLen]
		pos += valLen

		if name == "" {
			continue
		}
		strVal := string(val)
		r.Attrs[name] = strVal
		switch name {
		case "requesting-user-name":
			r.UserName = strVal
		case "job-name", "document-name":
			if r.JobName == "" {
				r.JobName = strVal
			}
		case "document-format":
			r.DocFormat = strVal
		case "printer-uri":
			r.PrinterURI = strVal
		case "job-id":
			if len(val) == 4 {
				r.JobID = int(binary.BigEndian.Uint32(val))
			}
		}
	}

	var docData []byte
	if pos < len(data) {
		docData = data[pos:]
		// Remove zeros no final
		for len(docData) > 0 && docData[len(docData)-1] == 0 {
			docData = docData[:len(docData)-1]
		}
	}
	return r, docData, nil
}

func (s *Server) handlePrintJob(req *IPPRequest, docData []byte) []byte {
	jobID := s.saveJob(req, docData)
	return s.buildJobResponse(STATUS_OK, req.RequestID, jobID, "completed")
}

func (s *Server) handleCreateJob(req *IPPRequest) []byte {
	job := &config.Job{
		Name:       req.JobName,
		User:       req.UserName,
		State:      "pending",
		Format:     req.DocFormat,
		ReceivedAt: time.Now().Format("02/01/2006 15:04:05"),
	}
	if job.Name == "" {
		job.Name = fmt.Sprintf("Job-%d", time.Now().UnixMilli())
	}
	s.Jobs.Add(job)
	s.Hub.Broadcast("job_created", map[string]interface{}{
		"id": job.ID, "name": job.Name, "state": job.State,
	})
	return s.buildJobResponse(STATUS_OK, req.RequestID, job.ID, "pending")
}

func (s *Server) handleSendDocument(req *IPPRequest, docData []byte) []byte {
	jobID := s.saveJob(req, docData)
	return s.buildJobResponse(STATUS_OK, req.RequestID, jobID, "completed")
}

func (s *Server) saveJob(req *IPPRequest, docData []byte) int {
	jobName := req.JobName
	if jobName == "" {
		jobName = fmt.Sprintf("Job-%d", time.Now().UnixMilli())
	}

	job := &config.Job{
		Name:       jobName,
		User:       req.UserName,
		State:      "processing",
		Format:     req.DocFormat,
		Data:       docData,
		Size:       len(docData),
		ReceivedAt: time.Now().Format("02/01/2006 15:04:05"),
	}
	s.Jobs.Add(job)

	// Salvar raw
	ext := formatExt(req.DocFormat)
	fname := fmt.Sprintf("job_%03d_%s%s", job.ID, sanitize(jobName), ext)
	fpath := filepath.Join(s.cfg.OutputDir, fname)
	if err := os.WriteFile(fpath, docData, 0644); err != nil {
		log.Printf("[IPP] Erro ao salvar raw: %v", err)
	} else {
		job.FilePath = fpath
	}

	log.Printf("[IPP] Job #%d recebido: %q (%s, %s)", job.ID, jobName, req.DocFormat, render.HumanSize(len(docData)))

	// Broadcast imediato
	s.Hub.Broadcast("job_received", map[string]interface{}{
		"id": job.ID, "name": job.Name, "user": job.User,
		"format": job.Format, "size": job.Size,
		"state": "processing", "received_at": job.ReceivedAt,
	})

	// Render assíncrono
	go func() {
		s.renderer.Render(job)
		s.Jobs.UpdateState(job.ID, "completed")
		log.Printf("[IPP] Job #%d processado OK", job.ID)
		s.Hub.Broadcast("job_completed", map[string]interface{}{
			"id": job.ID, "name": job.Name,
			"has_html": s.renderer.HasHTML(job.ID),
		})
	}()

	return job.ID
}

func (s *Server) handleGetPrinterAttrs(req *IPPRequest) []byte {
	buf := &bytes.Buffer{}
	writeIPPHeader(buf, STATUS_OK, req.RequestID)
	buf.WriteByte(TAG_OPERATION_GROUP)
	writeAttr(buf, TAG_CHARSET, "attributes-charset", "utf-8")
	writeAttr(buf, TAG_NATURAL_LANG, "attributes-natural-language", "pt-br")
	buf.WriteByte(TAG_PRINTER_GROUP)

	printerURI := fmt.Sprintf("ipp://localhost:%d/printers/%s", s.cfg.IPPPort, s.cfg.PrinterName)
	writeAttr(buf, TAG_URI, "printer-uri-supported", printerURI)
	writeAttr(buf, TAG_KEYWORD, "uri-security-supported", "none")
	writeAttr(buf, TAG_KEYWORD, "uri-authentication-supported", "none")
	writeAttr(buf, TAG_NAME_WITHOUT_LANG, "printer-name", s.cfg.PrinterName)
	writeAttr(buf, TAG_TEXT_WITHOUT_LANG, "printer-info", "Virtual Thermal Printer - Emulador de Cupom Fiscal")
	writeAttr(buf, TAG_TEXT_WITHOUT_LANG, "printer-make-and-model", "Virtual Thermal Printer v"+s.cfg.Version)
	writeAttr(buf, TAG_URI, "printer-icons", fmt.Sprintf("http://localhost:%d/icon.png", s.cfg.WebPort))
	writeAttrInt(buf, TAG_ENUM, "printer-state", 3) // idle
	writeAttr(buf, TAG_KEYWORD, "printer-state-reasons", "none")
	writeAttrBool(buf, "printer-is-accepting-jobs", true)
	writeAttr(buf, TAG_CHARSET, "charset-configured", "utf-8")
	writeAttr(buf, TAG_CHARSET, "charset-supported", "utf-8")
	writeAttr(buf, TAG_NATURAL_LANG, "natural-language-configured", "pt-br")
	writeAttr(buf, TAG_NATURAL_LANG, "generated-natural-language-supported", "pt-br")
	writeAttr(buf, TAG_MIME_TYPE, "document-format-default", "application/octet-stream")

	formats := []string{
		"application/octet-stream", "application/pdf", "application/postscript",
		"text/plain", "image/jpeg", "image/png",
		"application/vnd.cups-raster", "application/vnd.cups-pdf",
	}
	writeAttr(buf, TAG_MIME_TYPE, "document-format-supported", formats[0])
	for _, f := range formats[1:] {
		writeAttrAdditional(buf, TAG_MIME_TYPE, f)
	}

	writeAttrBool(buf, "color-supported", false)
	writeAttrInt(buf, TAG_INTEGER, "copies-default", 1)
	writeAttrRange(buf, "copies-supported", 1, 1)
	writeAttrRes(buf, "printer-resolution-default", 203, 203, 3)
	writeAttrRes(buf, "printer-resolution-supported", 203, 203, 3)
	writeAttr(buf, TAG_KEYWORD, "media-default", paperWidthToMedia(s.cfg.PaperWidth))
	writeAttr(buf, TAG_KEYWORD, "media-supported", paperWidthToMedia(s.cfg.PaperWidth))
	writeAttr(buf, TAG_KEYWORD, "print-quality-default", "draft")
	writeAttr(buf, TAG_KEYWORD, "sides-default", "one-sided")
	writeAttr(buf, TAG_KEYWORD, "sides-supported", "one-sided")
	writeAttrInt(buf, TAG_INTEGER, "queued-job-count", len(s.Jobs.All()))
	writeAttrBool(buf, "pdl-override-supported", false)
	writeAttrInt(buf, TAG_INTEGER, "printer-up-time", int(time.Now().Unix()))
	buf.WriteByte(TAG_END_ATTRS)
	return buf.Bytes()
}

func (s *Server) handleGetJobs(req *IPPRequest) []byte {
	buf := &bytes.Buffer{}
	writeIPPHeader(buf, STATUS_OK, req.RequestID)
	buf.WriteByte(TAG_OPERATION_GROUP)
	writeAttr(buf, TAG_CHARSET, "attributes-charset", "utf-8")
	writeAttr(buf, TAG_NATURAL_LANG, "attributes-natural-language", "pt-br")
	printerURI := fmt.Sprintf("ipp://localhost:%d/printers/%s", s.cfg.IPPPort, s.cfg.PrinterName)
	for _, job := range s.Jobs.All() {
		buf.WriteByte(TAG_JOB_GROUP)
		writeAttrInt(buf, TAG_INTEGER, "job-id", job.ID)
		writeAttr(buf, TAG_NAME_WITHOUT_LANG, "job-name", job.Name)
		writeAttr(buf, TAG_KEYWORD, "job-state", jobStateCode(job.State))
		writeAttr(buf, TAG_KEYWORD, "job-state-reasons", "none")
		writeAttr(buf, TAG_NAME_WITHOUT_LANG, "job-originating-user-name", job.User)
		writeAttr(buf, TAG_URI, "job-printer-uri", printerURI)
	}
	buf.WriteByte(TAG_END_ATTRS)
	return buf.Bytes()
}

func (s *Server) handleGetJobAttrs(req *IPPRequest) []byte {
	buf := &bytes.Buffer{}
	writeIPPHeader(buf, STATUS_OK, req.RequestID)
	buf.WriteByte(TAG_OPERATION_GROUP)
	writeAttr(buf, TAG_CHARSET, "attributes-charset", "utf-8")
	writeAttr(buf, TAG_NATURAL_LANG, "attributes-natural-language", "pt-br")
	jobs := s.Jobs.All()
	if len(jobs) > 0 {
		job := jobs[len(jobs)-1]
		buf.WriteByte(TAG_JOB_GROUP)
		writeAttrInt(buf, TAG_INTEGER, "job-id", job.ID)
		writeAttr(buf, TAG_NAME_WITHOUT_LANG, "job-name", job.Name)
		writeAttr(buf, TAG_KEYWORD, "job-state", jobStateCode(job.State))
		writeAttr(buf, TAG_KEYWORD, "job-state-reasons", "none")
	}
	buf.WriteByte(TAG_END_ATTRS)
	return buf.Bytes()
}

func (s *Server) buildJobResponse(status uint16, reqID uint32, jobID int, state string) []byte {
	buf := &bytes.Buffer{}
	writeIPPHeader(buf, status, reqID)
	buf.WriteByte(TAG_OPERATION_GROUP)
	writeAttr(buf, TAG_CHARSET, "attributes-charset", "utf-8")
	writeAttr(buf, TAG_NATURAL_LANG, "attributes-natural-language", "pt-br")
	writeAttr(buf, TAG_TEXT_WITHOUT_LANG, "status-message", "successful-ok")
	buf.WriteByte(TAG_JOB_GROUP)
	writeAttrInt(buf, TAG_INTEGER, "job-id", jobID)
	printerURI := fmt.Sprintf("ipp://localhost:%d/printers/%s", s.cfg.IPPPort, s.cfg.PrinterName)
	writeAttr(buf, TAG_URI, "job-uri", fmt.Sprintf("%s/jobs/%d", printerURI, jobID))
	writeAttr(buf, TAG_URI, "job-printer-uri", printerURI)
	writeAttr(buf, TAG_KEYWORD, "job-state", jobStateCode(state))
	writeAttr(buf, TAG_KEYWORD, "job-state-reasons", "none")
	buf.WriteByte(TAG_END_ATTRS)
	return buf.Bytes()
}

func (s *Server) buildSimpleResponse(status uint16, reqID uint32, jobID int) []byte {
	buf := &bytes.Buffer{}
	writeIPPHeader(buf, status, reqID)
	buf.WriteByte(TAG_OPERATION_GROUP)
	writeAttr(buf, TAG_CHARSET, "attributes-charset", "utf-8")
	writeAttr(buf, TAG_NATURAL_LANG, "attributes-natural-language", "pt-br")
	if status == STATUS_OK {
		writeAttr(buf, TAG_TEXT_WITHOUT_LANG, "status-message", "successful-ok")
	}
	if jobID > 0 {
		buf.WriteByte(TAG_JOB_GROUP)
		writeAttrInt(buf, TAG_INTEGER, "job-id", jobID)
		writeAttr(buf, TAG_KEYWORD, "job-state", jobStateCode("completed"))
		writeAttr(buf, TAG_KEYWORD, "job-state-reasons", "none")
	}
	buf.WriteByte(TAG_END_ATTRS)
	return buf.Bytes()
}

// Helpers de serialização IPP

func writeIPPHeader(buf *bytes.Buffer, status uint16, reqID uint32) {
	buf.WriteByte(IPP_VERSION_MAJOR)
	buf.WriteByte(IPP_VERSION_MINOR)
	b2 := make([]byte, 2)
	binary.BigEndian.PutUint16(b2, status)
	buf.Write(b2)
	b4 := make([]byte, 4)
	binary.BigEndian.PutUint32(b4, reqID)
	buf.Write(b4)
}

func writeAttr(buf *bytes.Buffer, tag byte, name, value string) {
	buf.WriteByte(tag)
	writeLen(buf, len(name))
	buf.WriteString(name)
	writeLen(buf, len(value))
	buf.WriteString(value)
}

func writeAttrAdditional(buf *bytes.Buffer, tag byte, value string) {
	buf.WriteByte(tag)
	writeLen(buf, 0)
	writeLen(buf, len(value))
	buf.WriteString(value)
}

func writeAttrInt(buf *bytes.Buffer, tag byte, name string, value int) {
	buf.WriteByte(tag)
	writeLen(buf, len(name))
	buf.WriteString(name)
	writeLen(buf, 4)
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(value))
	buf.Write(b)
}

func writeAttrBool(buf *bytes.Buffer, name string, value bool) {
	buf.WriteByte(TAG_BOOLEAN)
	writeLen(buf, len(name))
	buf.WriteString(name)
	writeLen(buf, 1)
	if value {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
}

func writeAttrRange(buf *bytes.Buffer, name string, lower, upper int) {
	buf.WriteByte(TAG_RANGE_OF_INT)
	writeLen(buf, len(name))
	buf.WriteString(name)
	writeLen(buf, 8)
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(lower))
	buf.Write(b)
	binary.BigEndian.PutUint32(b, uint32(upper))
	buf.Write(b)
}

func writeAttrRes(buf *bytes.Buffer, name string, xres, yres, units int) {
	buf.WriteByte(TAG_RESOLUTION)
	writeLen(buf, len(name))
	buf.WriteString(name)
	writeLen(buf, 9)
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(xres))
	buf.Write(b)
	binary.BigEndian.PutUint32(b, uint32(yres))
	buf.Write(b)
	buf.WriteByte(byte(units))
}

func writeLen(buf *bytes.Buffer, n int) {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(n))
	buf.Write(b)
}

func jobStateCode(state string) string {
	switch state {
	case "pending":
		return "\x00\x03"
	case "processing":
		return "\x00\x05"
	case "completed":
		return "\x00\x09"
	case "aborted":
		return "\x00\x08"
	default:
		return "\x00\x09"
	}
}

func paperWidthToMedia(w string) string {
	switch w {
	case "58":
		return "custom_58x200mm_58x200mm"
	case "110":
		return "custom_110x200mm_110x200mm"
	default:
		return "custom_80x200mm_80x200mm"
	}
}

func formatExt(format string) string {
	switch {
	case strings.Contains(format, "pdf"):
		return ".pdf"
	case strings.Contains(format, "postscript"):
		return ".ps"
	case strings.Contains(format, "jpeg"):
		return ".jpg"
	case strings.Contains(format, "png"):
		return ".png"
	case strings.Contains(format, "text"):
		return ".txt"
	default:
		return ".bin"
	}
}

func sanitize(s string) string {
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
			b.WriteRune(c)
		} else {
			b.WriteRune('_')
		}
	}
	r := b.String()
	if len(r) > 40 {
		r = r[:40]
	}
	return r
}
