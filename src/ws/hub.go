package ws

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
)

// Hub gerencia todas as conexões WebSocket ativas
type Hub struct {
	mu      sync.RWMutex
	clients map[*Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*Conn]struct{})}
}

// Broadcast envia um evento JSON para todos os clientes conectados
func (h *Hub) Broadcast(event string, data interface{}) {
	msg, err := json.Marshal(map[string]interface{}{
		"event": event,
		"data":  data,
	})
	if err != nil {
		return
	}
	frame := encodeFrame(msg)

	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		c.write(frame)
	}
}

func (h *Hub) add(c *Conn) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) remove(c *Conn) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

// Count retorna o número de clientes conectados
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Conn representa uma conexão WebSocket
type Conn struct {
	conn net.Conn
	mu   sync.Mutex
}

func (c *Conn) write(data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn.Write(data)
}

func (c *Conn) close() {
	c.conn.Close()
}

// Upgrade faz o handshake WebSocket e registra o cliente no Hub
func (h *Hub) Upgrade(w http.ResponseWriter, r *http.Request) {
	if !isWebSocketUpgrade(r) {
		http.Error(w, "Not a WebSocket upgrade", http.StatusBadRequest)
		return
	}

	key := r.Header.Get("Sec-Websocket-Key")
	if key == "" {
		http.Error(w, "Missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	accept := computeAccept(key)

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijack not supported", http.StatusInternalServerError)
		return
	}

	netConn, buf, err := hj.Hijack()
	if err != nil {
		return
	}

	// Drena buffer pendente
	_ = buf

	// Envia resposta de handshake
	resp := fmt.Sprintf(
		"HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: %s\r\n\r\n",
		accept,
	)
	netConn.Write([]byte(resp))

	c := &Conn{conn: netConn}
	h.add(c)

	// Loop de leitura (para detectar desconexão)
	go func() {
		defer func() {
			h.remove(c)
			c.close()
		}()
		buf := make([]byte, 256)
		for {
			_, err := netConn.Read(buf)
			if err != nil {
				return
			}
		}
	}()
}

// encodeFrame cria um frame WebSocket binário (opcode text, sem máscara)
func encodeFrame(payload []byte) []byte {
	n := len(payload)
	var frame []byte

	// FIN=1, opcode=1 (text)
	frame = append(frame, 0x81)

	if n < 126 {
		frame = append(frame, byte(n))
	} else if n < 65536 {
		frame = append(frame, 126)
		frame = append(frame, byte(n>>8), byte(n))
	} else {
		frame = append(frame, 127)
		for i := 7; i >= 0; i-- {
			frame = append(frame, byte(n>>(uint(i)*8)))
		}
	}

	frame = append(frame, payload...)
	return frame
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket" &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func computeAccept(key string) string {
	const magic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.New()
	h.Write([]byte(key + magic))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
