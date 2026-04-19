package ui

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"virtual-printer/config"
	"virtual-printer/ipp"
	"virtual-printer/render"
	"virtual-printer/ws"
)

type WebUI struct {
	cfg      *config.Config
	server   *ipp.Server
	renderer *render.Renderer
	hub      *ws.Hub
}

func NewWebUI(cfg *config.Config, server *ipp.Server, hub *ws.Hub) *WebUI {
	return &WebUI{
		cfg:      cfg,
		server:   server,
		renderer: render.NewRenderer(cfg),
		hub:      hub,
	}
}

func (u *WebUI) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", u.handleDashboard)
	mux.HandleFunc("/ws", u.handleWS)
	mux.HandleFunc("/api/jobs", u.handleAPIJobs)
	mux.HandleFunc("/api/job/", u.handleAPIJobDetail)
	mux.HandleFunc("/api/clear", u.handleAPIClear)
	mux.HandleFunc("/api/status", u.handleAPIStatus)
	mux.HandleFunc("/api/config", u.handleAPIConfig)
	mux.HandleFunc("/jobs/", u.handleViewJob)
	mux.HandleFunc("/icon.png", u.handleIcon)

	addr := fmt.Sprintf(":%d", u.cfg.WebPort)
	log.Printf("Interface web em http://localhost%s", addr)
	return http.ListenAndServe(addr, mux)
}

func (u *WebUI) handleWS(w http.ResponseWriter, r *http.Request) {
	u.hub.Upgrade(w, r)
}

func (u *WebUI) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"printer":     u.cfg.PrinterName,
		"paper_width": u.cfg.PaperWidth,
		"ipp_port":    u.cfg.IPPPort,
		"web_port":    u.cfg.WebPort,
		"jobs":        len(u.server.Jobs.All()),
		"ws_clients":  u.hub.Count(),
		"time":        time.Now().Format("02/01/2006 15:04:05"),
		"version":     u.cfg.Version,
		"ipp_url":     fmt.Sprintf("ipp://localhost:%d/printers/%s", u.cfg.IPPPort, u.cfg.PrinterName),
	})
}

func (u *WebUI) handleAPIConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"printer_name": u.cfg.PrinterName,
		"paper_width":  u.cfg.PaperWidth,
		"ipp_port":     u.cfg.IPPPort,
		"web_port":     u.cfg.WebPort,
		"version":      u.cfg.Version,
		"linux_cmd":    fmt.Sprintf("sudo lpadmin -p %s -E -v ipp://localhost:%d/printers/%s -m everywhere", u.cfg.PrinterName, u.cfg.IPPPort, u.cfg.PrinterName),
		"windows_url":  fmt.Sprintf("http://localhost:%d/printers/%s", u.cfg.IPPPort, u.cfg.PrinterName),
		"test_cmd":     fmt.Sprintf("echo 'Teste' | lp -d %s", u.cfg.PrinterName),
	})
}

func (u *WebUI) handleAPIJobs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	jobs := u.server.Jobs.All()
	type jobDTO struct {
		ID         int    `json:"id"`
		Name       string `json:"name"`
		User       string `json:"user"`
		State      string `json:"state"`
		Format     string `json:"format"`
		Size       int    `json:"size"`
		SizeHuman  string `json:"size_human"`
		ReceivedAt string `json:"received_at"`
		HasHTML    bool   `json:"has_html"`
	}
	result := make([]jobDTO, 0, len(jobs))
	for _, j := range jobs {
		result = append(result, jobDTO{
			ID:         j.ID,
			Name:       j.Name,
			User:       j.User,
			State:      j.State,
			Format:     j.Format,
			Size:       j.Size,
			SizeHuman:  render.HumanSize(j.Size),
			ReceivedAt: j.ReceivedAt,
			HasHTML:    u.renderer.HasHTML(j.ID),
		})
	}
	json.NewEncoder(w).Encode(result)
}

func (u *WebUI) handleAPIJobDetail(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/job/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "ID invalido", http.StatusBadRequest)
		return
	}
	job := u.server.Jobs.Get(id)
	if job == nil {
		http.Error(w, "Job nao encontrado", http.StatusNotFound)
		return
	}
	receipt := u.renderer.GetReceiptText(job)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":          job.ID,
		"name":        job.Name,
		"user":        job.User,
		"state":       job.State,
		"format":      job.Format,
		"size":        job.Size,
		"size_human":  render.HumanSize(job.Size),
		"received_at": job.ReceivedAt,
		"receipt":     receipt,
		"has_html":    u.renderer.HasHTML(job.ID),
	})
}

func (u *WebUI) handleAPIClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	u.server.Jobs.Clear()
	u.hub.Broadcast("jobs_cleared", nil)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})
}

func (u *WebUI) handleViewJob(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/jobs/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "ID invalido", http.StatusBadRequest)
		return
	}
	data, err := u.renderer.GetReceiptHTML(id)
	if err != nil {
		http.Error(w, "Job nao encontrado ou HTML nao gerado", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func (u *WebUI) handleIcon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Write([]byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00,
		0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x62, 0x00, 0x00, 0x00, 0x02,
		0x00, 0x01, 0xE2, 0x21, 0xBC, 0x33, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
		0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
	})
}

func (u *WebUI) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, dashboardHTML())
}

func dashboardHTML() string {
	return `<!DOCTYPE html>
<html lang="pt-br">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Virtual Thermal Printer</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#111827;color:#f3f4f6;min-height:100vh}
header{background:#1f2937;border-bottom:1px solid #374151;padding:14px 24px;display:flex;align-items:center;gap:12px;position:sticky;top:0;z-index:10}
header h1{font-size:16px;font-weight:600;color:#f9fafb}
.badge{padding:2px 9px;border-radius:12px;font-size:11px;font-weight:500}
.badge-blue{background:#1e3a5f;color:#60a5fa}
.badge-green{background:#14532d;color:#4ade80}
.badge-yellow{background:#451a03;color:#fbbf24}
.badge-red{background:#450a0a;color:#f87171}
.badge-gray{background:#1f2937;color:#9ca3af}
.container{max-width:1200px;margin:0 auto;padding:20px}
.grid4{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:12px;margin-bottom:20px}
.card{background:#1f2937;border:1px solid #374151;border-radius:10px;padding:16px}
.card h3{font-size:11px;color:#6b7280;text-transform:uppercase;letter-spacing:.06em;margin-bottom:6px}
.card .val{font-size:24px;font-weight:700;color:#f9fafb;font-variant-numeric:tabular-nums}
.card .sub{font-size:12px;color:#4b5563;margin-top:3px}
.section{background:#1f2937;border:1px solid #374151;border-radius:10px;padding:16px;margin-bottom:16px}
.section-header{display:flex;align-items:center;gap:10px;margin-bottom:14px}
.section-header h2{font-size:14px;font-weight:600;color:#e5e7eb}
.setup-block{background:#111827;border:1px solid #374151;border-radius:8px;padding:12px;margin-bottom:10px}
.setup-block label{font-size:11px;color:#6b7280;display:block;margin-bottom:6px;text-transform:uppercase;letter-spacing:.04em}
.setup-block code{font-family:'Courier New',monospace;font-size:12px;color:#a5f3fc;display:block;word-break:break-all;line-height:1.6}
.copy-btn{margin-top:6px;padding:3px 10px;font-size:11px;border-radius:5px;border:1px solid #374151;background:#374151;color:#d1d5db;cursor:pointer}
.copy-btn:hover{background:#4b5563}
table{width:100%;border-collapse:collapse;font-size:13px}
th{text-align:left;padding:8px 10px;border-bottom:1px solid #374151;font-size:11px;color:#6b7280;text-transform:uppercase;letter-spacing:.04em;font-weight:500}
td{padding:9px 10px;border-bottom:1px solid #1f2937;vertical-align:middle;color:#d1d5db}
tr:hover td{background:#374151}
.toolbar{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.btn{padding:6px 14px;border-radius:7px;border:1px solid #374151;background:#374151;color:#d1d5db;cursor:pointer;font-size:12px;font-weight:500;transition:background .15s}
.btn:hover{background:#4b5563}
.btn-primary{background:#1d4ed8;border-color:#1d4ed8;color:#fff}
.btn-primary:hover{background:#1e40af}
.btn-danger{background:#7f1d1d;border-color:#7f1d1d;color:#fca5a5}
.btn-danger:hover{background:#991b1b}
.ws-dot{width:8px;height:8px;border-radius:50%;background:#374151;display:inline-block;margin-right:6px;transition:background .3s}
.ws-dot.on{background:#4ade80}
.ws-dot.recv{background:#fbbf24}
#log-area{max-height:120px;overflow-y:auto;background:#111827;border:1px solid #374151;border-radius:7px;padding:8px;font-family:monospace;font-size:11px;color:#6b7280;margin-top:10px}
.log-entry{padding:2px 0;border-bottom:1px solid #1f2937;line-height:1.5}
.log-entry.new{color:#4ade80}
#modal-overlay{display:none;position:fixed;inset:0;background:rgba(0,0,0,.7);z-index:50;align-items:center;justify-content:center;padding:20px}
#modal-overlay.open{display:flex}
.modal{background:#1f2937;border:1px solid #374151;border-radius:12px;padding:20px;max-width:520px;width:100%;max-height:85vh;display:flex;flex-direction:column}
.modal-head{display:flex;align-items:center;justify-content:space-between;margin-bottom:12px}
.modal-head h3{font-size:15px;font-weight:600;color:#f9fafb}
.modal-close{background:none;border:none;color:#6b7280;cursor:pointer;font-size:20px;padding:0 4px}
.modal-close:hover{color:#f9fafb}
.receipt-pre{font-family:'Courier New',monospace;font-size:11px;background:#111827;border:1px solid #374151;border-radius:6px;padding:12px;overflow-y:auto;flex:1;white-space:pre;line-height:1.6;color:#d1fae5}
.empty-msg{text-align:center;color:#4b5563;padding:40px 0;font-size:14px}
</style>
</head>
<body>
<header>
  <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="#60a5fa" stroke-width="2"><rect x="2" y="7" width="20" height="14" rx="2"/><path d="M16 7V5a2 2 0 0 0-4 0v2"/><line x1="12" y1="12" x2="12" y2="16"/></svg>
  <h1 id="h-name">Virtual Thermal Printer</h1>
  <span class="badge badge-blue" id="h-ver">v1.0.0</span>
  <span class="badge badge-gray" id="h-paper">80mm</span>
  <span style="margin-left:auto;display:flex;align-items:center;font-size:12px;color:#6b7280">
    <span class="ws-dot" id="ws-dot"></span><span id="ws-status">conectando...</span>
  </span>
</header>

<div class="container">
  <div class="grid4">
    <div class="card"><h3>Jobs recebidos</h3><div class="val" id="s-jobs">0</div><div class="sub">total na sessão</div></div>
    <div class="card"><h3>Porta IPP</h3><div class="val" id="s-ipp">631</div><div class="sub">IPP/2.0 RFC 8011</div></div>
    <div class="card"><h3>Clientes WS</h3><div class="val" id="s-ws">0</div><div class="sub">painéis abertos</div></div>
    <div class="card"><h3>Status</h3><div class="val" style="font-size:15px;padding-top:5px"><span class="ws-dot on"></span>Online</div><div class="sub" id="s-time">--</div></div>
  </div>

  <div class="section">
    <div class="section-header"><h2>Instalação</h2></div>
    <div class="setup-block">
      <label>Linux — CUPS (execute como root)</label>
      <code id="cmd-linux">carregando...</code>
      <button class="copy-btn" onclick="copy('cmd-linux')">Copiar</button>
    </div>
    <div class="setup-block">
      <label>Windows — URL para impressora de rede</label>
      <code id="cmd-windows">carregando...</code>
      <button class="copy-btn" onclick="copy('cmd-windows')">Copiar</button>
    </div>
    <div class="setup-block">
      <label>Testar impressão (Linux)</label>
      <code id="cmd-test">carregando...</code>
      <button class="copy-btn" onclick="copy('cmd-test')">Copiar</button>
    </div>
    <div class="setup-block">
      <label>Testar com ESC/POS direto (Python)</label>
      <code>python3 -c "import socket; s=socket.socket(); s.connect(('localhost',9100)); s.send(b'\x1b\x40Teste ESC/POS!\x0a\x1dV\x41\x00'); s.close()"</code>
    </div>
  </div>

  <div class="section">
    <div class="section-header" style="margin-bottom:10px">
      <h2>Fila de jobs</h2>
      <div class="toolbar" style="margin-left:auto">
        <button class="btn" onclick="loadJobs()">Atualizar</button>
        <button class="btn btn-danger" onclick="clearJobs()">Limpar tudo</button>
      </div>
    </div>
    <div id="log-area"></div>
    <div style="margin-top:14px">
      <table>
        <thead><tr><th>#</th><th>Nome do job</th><th>Usuário</th><th>Formato</th><th>Tamanho</th><th>Recebido</th><th>Status</th><th></th></tr></thead>
        <tbody id="jobs-table"><tr><td colspan="8"><div class="empty-msg">Nenhum job recebido ainda.<br>Envie algo para imprimir!</div></td></tr></tbody>
      </table>
    </div>
  </div>
</div>

<div id="modal-overlay">
  <div class="modal">
    <div class="modal-head">
      <h3 id="modal-title">Cupom</h3>
      <button class="modal-close" onclick="closeModal()">&#x2715;</button>
    </div>
    <div class="receipt-pre" id="modal-receipt"></div>
    <div style="margin-top:10px;display:flex;gap:8px">
      <a id="modal-html-link" href="#" target="_blank" style="display:none"><button class="btn btn-primary">Ver HTML</button></a>
      <button class="btn" onclick="closeModal()">Fechar</button>
    </div>
  </div>
</div>

<script>
var cfg = {};
var ws;
var reconnectTimer;

function badgeClass(s) {
  return {completed:'badge-green',processing:'badge-yellow',pending:'badge-blue',aborted:'badge-red'}[s]||'badge-gray';
}

function stateLabel(s) {
  return '<span class="badge '+badgeClass(s)+'">'+s+'</span>';
}

function copy(id) {
  var el = document.getElementById(id);
  if (!el) return;
  navigator.clipboard.writeText(el.textContent).then(function(){
    el.style.color='#4ade80';
    setTimeout(function(){el.style.color='';},1000);
  });
}

function addLog(msg, isNew) {
  var area = document.getElementById('log-area');
  var d = document.createElement('div');
  d.className = 'log-entry' + (isNew ? ' new' : '');
  d.textContent = new Date().toLocaleTimeString('pt-BR') + ' — ' + msg;
  area.insertBefore(d, area.firstChild);
  if (area.children.length > 50) area.removeChild(area.lastChild);
}

function connectWS() {
  var proto = location.protocol === 'https:' ? 'wss' : 'ws';
  ws = new WebSocket(proto + '://' + location.host + '/ws');
  ws.onopen = function() {
    document.getElementById('ws-dot').className = 'ws-dot on';
    document.getElementById('ws-status').textContent = 'ao vivo';
    clearTimeout(reconnectTimer);
    addLog('WebSocket conectado');
  };
  ws.onclose = function() {
    document.getElementById('ws-dot').className = 'ws-dot';
    document.getElementById('ws-status').textContent = 'reconectando...';
    reconnectTimer = setTimeout(connectWS, 3000);
  };
  ws.onmessage = function(e) {
    try {
      var msg = JSON.parse(e.data);
      handleWSEvent(msg.event, msg.data);
    } catch(err) {}
  };
}

function handleWSEvent(event, data) {
  var dot = document.getElementById('ws-dot');
  dot.className = 'ws-dot recv';
  setTimeout(function(){dot.className='ws-dot on';}, 400);

  if (event === 'job_received' || event === 'job_completed') {
    addLog('Job #' + data.id + ' — ' + (data.name||'sem nome') + ' [' + event + ']', true);
    loadJobs();
  } else if (event === 'job_created') {
    addLog('Job criado: #' + data.id, true);
  } else if (event === 'jobs_cleared') {
    addLog('Fila limpa');
    loadJobs();
  }
}

async function loadStatus() {
  try {
    var s = await (await fetch('/api/status')).json();
    cfg = await (await fetch('/api/config')).json();
    document.getElementById('h-name').textContent = s.printer;
    document.getElementById('h-ver').textContent = 'v' + s.version;
    document.getElementById('h-paper').textContent = s.paper_width + 'mm';
    document.getElementById('s-jobs').textContent = s.jobs;
    document.getElementById('s-ipp').textContent = s.ipp_port;
    document.getElementById('s-ws').textContent = s.ws_clients;
    document.getElementById('s-time').textContent = s.time;
    document.getElementById('cmd-linux').textContent = cfg.linux_cmd;
    document.getElementById('cmd-windows').textContent = cfg.windows_url;
    document.getElementById('cmd-test').textContent = cfg.test_cmd;
  } catch(e) {}
}

async function loadJobs() {
  try {
    var jobs = await (await fetch('/api/jobs')).json();
    var tb = document.getElementById('jobs-table');
    document.getElementById('s-jobs').textContent = jobs ? jobs.length : 0;
    if (!jobs || !jobs.length) {
      tb.innerHTML = '<tr><td colspan="8"><div class="empty-msg">Nenhum job recebido ainda.<br>Envie algo para imprimir!</div></td></tr>';
      return;
    }
    tb.innerHTML = jobs.slice().reverse().map(function(j) {
      var fmt = (j.format||'').replace('application/','').replace('image/','img/');
      var actions = '<button class="btn btn-primary" onclick="showJob(' + j.id + ')">Ver</button>';
      if (j.has_html) actions += ' <a href="/jobs/' + j.id + '" target="_blank"><button class="btn">HTML</button></a>';
      return '<tr><td style="color:#6b7280;font-size:12px">#' + j.id + '</td>' +
        '<td style="font-weight:500;color:#f9fafb">' + (j.name||'-') + '</td>' +
        '<td style="color:#9ca3af">' + (j.user||'-') + '</td>' +
        '<td style="font-size:11px;color:#6b7280">' + fmt + '</td>' +
        '<td style="font-size:12px">' + (j.size_human||j.size+'B') + '</td>' +
        '<td style="font-size:12px;color:#6b7280">' + (j.received_at||'-') + '</td>' +
        '<td>' + stateLabel(j.state) + '</td>' +
        '<td>' + actions + '</td></tr>';
    }).join('');
  } catch(e) { console.error(e); }
}

async function showJob(id) {
  try {
    var j = await (await fetch('/api/job/' + id)).json();
    document.getElementById('modal-title').textContent = 'Job #' + j.id + ' — ' + (j.name||'');
    document.getElementById('modal-receipt').textContent = j.receipt || '(sem conteudo de texto)';
    var link = document.getElementById('modal-html-link');
    if (j.has_html) {
      link.href = '/jobs/' + j.id;
      link.style.display = '';
    } else {
      link.style.display = 'none';
    }
    document.getElementById('modal-overlay').classList.add('open');
  } catch(e) { console.error(e); }
}

function closeModal() {
  document.getElementById('modal-overlay').classList.remove('open');
}

async function clearJobs() {
  if (!confirm('Limpar todos os jobs da sessão?')) return;
  await fetch('/api/clear', { method: 'POST' });
  loadJobs();
}

document.getElementById('modal-overlay').addEventListener('click', function(e) {
  if (e.target === this) closeModal();
});

connectWS();
loadStatus();
loadJobs();
setInterval(loadStatus, 5000);
</script>
</body>
</html>`
}
