package cmd

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

const (
	scrollbackLimit = 256 * 1024 // bytes of scrollback kept per session
	defaultCols     = 80
	defaultRows     = 24
)

// Session is a live PTY on the agent. It survives web-client disconnects:
// output keeps accumulating in the scrollback buffer and is replayed on reattach.
type Session struct {
	id   string
	cmd  *exec.Cmd
	ptmx *os.File

	mu         sync.Mutex
	scrollback []byte
	cols       int
	rows       int
}

func (s *Session) appendScrollback(p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scrollback = append(s.scrollback, p...)
	if len(s.scrollback) > scrollbackLimit {
		s.scrollback = s.scrollback[len(s.scrollback)-scrollbackLimit:]
	}
}

func (s *Session) snapshot() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]byte, len(s.scrollback))
	copy(out, s.scrollback)
	return out
}

func (s *Session) setSize(cols, rows int) {
	s.mu.Lock()
	s.cols, s.rows = cols, rows
	s.mu.Unlock()
	if s.ptmx != nil {
		pty.Setsize(s.ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	}
}

func (s *Session) size() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cols, s.rows
}

type Agent struct {
	Token  string
	Server string

	connMu sync.Mutex
	conn   *websocket.Conn

	writeMu sync.Mutex // serializes writes to the server connection

	sessMu   sync.Mutex
	sessions map[string]*Session

	setStatus func(string)
}

func NewAgent(token, server string) *Agent {
	return &Agent{
		Token:    token,
		Server:   server,
		sessions: make(map[string]*Session),
	}
}

func (a *Agent) connect() error {
	url := a.Server + "/api/ws/agent?token=" + a.Token
	a.setStatus("Connecting...")
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return err
	}
	a.connMu.Lock()
	a.conn = c
	a.connMu.Unlock()
	a.setStatus("Connected")
	return nil
}

func (a *Agent) setConn(c *websocket.Conn) {
	a.connMu.Lock()
	a.conn = c
	a.connMu.Unlock()
}

func (a *Agent) send(msg WsMessage) {
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	a.connMu.Lock()
	c := a.conn
	a.connMu.Unlock()
	if c != nil {
		c.WriteMessage(websocket.TextMessage, b)
	}
}

func (a *Agent) closeConn() {
	a.connMu.Lock()
	defer a.connMu.Unlock()
	if a.conn != nil {
		a.conn.Close()
	}
}

func (a *Agent) RunLoop() {
	resetInheritedSignals()
	for {
		if err := a.connect(); err != nil {
			a.setStatus("Error (Retrying in 5s)")
			time.Sleep(5 * time.Second)
			continue
		}

		for {
			a.connMu.Lock()
			c := a.conn
			a.connMu.Unlock()
			if c == nil {
				break
			}

			_, message, err := c.ReadMessage()
			if err != nil {
				a.setStatus("Disconnected")
				a.closeConn()
				break
			}

			var msg WsMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				continue
			}
			a.handleMessage(msg)
		}
	}
}

func (a *Agent) handleMessage(msg WsMessage) {
	switch msg.Type {
	case "start_session":
		a.startOrAttach(msg.SessionID)
	case "terminal_data":
		if s := a.getSession(msg.SessionID); s != nil && s.ptmx != nil {
			s.ptmx.Write([]byte(msg.Data))
		}
	case "resize":
		if s := a.getSession(msg.SessionID); s != nil {
			s.setSize(msg.Cols, msg.Rows)
		} else {
			// Remember the size for a session that may start later.
			s := &Session{id: msg.SessionID, cols: msg.Cols, rows: msg.Rows}
			a.sessMu.Lock()
			a.sessions[msg.SessionID] = s
			a.sessMu.Unlock()
		}
	case "restart_session":
		a.killSession(msg.SessionID, false)
		a.startOrAttach(msg.SessionID)
	case "kill_session":
		a.killSession(msg.SessionID, true)
	}
}

func (a *Agent) getSession(id string) *Session {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	return a.sessions[id]
}

// startOrAttach starts a fresh PTY or, if the session already exists,
// replays the scrollback so the client restores its view.
func (a *Agent) startOrAttach(id string) {
	a.sessMu.Lock()
	s, exists := a.sessions[id]
	if !exists {
		s = &Session{id: id, cols: defaultCols, rows: defaultRows}
		a.sessions[id] = s
	}
	running := exists && s.ptmx != nil
	a.sessMu.Unlock()

	if running {
		log.Printf("Reattaching session %s (replay %d bytes)", id, len(s.scrollback))
		a.send(WsMessage{Type: "replay", SessionID: id, Data: string(s.snapshot())})
		return
	}

	log.Printf("Starting new PTY for session %s", id)
	c := exec.Command("bash")
	c.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	)
	ptmx, err := pty.Start(c)
	if err != nil {
		log.Printf("Failed to start pty: %v", err)
		a.send(WsMessage{Type: "session_closed", SessionID: id, Data: "failed to start shell"})
		return
	}

	s.mu.Lock()
	s.cmd = c
	s.ptmx = ptmx
	s.scrollback = nil
	cols, rows := s.cols, s.rows
	s.mu.Unlock()
	pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})

	a.send(WsMessage{Type: "session_started", SessionID: id})

	go a.pumpOutput(s)
}

// pumpOutput forwards PTY output to the server and keeps the scrollback.
// It keeps UTF-8 sequences intact across read boundaries (TUI apps render
// box-drawing characters that must not be split).
func (a *Agent) pumpOutput(s *Session) {
	buf := make([]byte, 32*1024)
	var pending []byte
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			chunk := append(pending, buf[:n]...)
			// Hold back an incomplete trailing UTF-8 sequence:
			// emit the longest valid prefix, keep the rest for the next read.
			emit := chunk
			pending = nil
			if !utf8.Valid(chunk) {
				for i := len(chunk) - 1; i >= 0; i-- {
					if utf8.Valid(chunk[:i]) {
						emit = chunk[:i]
						pending = append([]byte(nil), chunk[i:]...)
						break
					}
				}
			}
			if len(emit) > 0 {
				s.appendScrollback(emit)
				a.send(WsMessage{Type: "terminal_data", SessionID: s.id, Data: string(emit)})
			}
		}
		if err != nil {
			// PTY closed (shell exited).
			a.sessMu.Lock()
			if cur, ok := a.sessions[s.id]; ok && cur == s {
				s.ptmx = nil
			}
			a.sessMu.Unlock()
			a.send(WsMessage{Type: "session_closed", SessionID: s.id})
			return
		}
	}
}

func (a *Agent) killSession(id string, notify bool) {
	a.sessMu.Lock()
	s, ok := a.sessions[id]
	if ok {
		delete(a.sessions, id)
	}
	a.sessMu.Unlock()

	if !ok {
		return
	}
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	if s.ptmx != nil {
		s.ptmx.Close()
	}
	log.Printf("Killed session %s", id)
	if notify {
		a.send(WsMessage{Type: "session_closed", SessionID: id})
	}
}

// closeAllSessions is used on shutdown.
func (a *Agent) closeAllSessions() {
	a.sessMu.Lock()
	ids := make([]string, 0, len(a.sessions))
	for id := range a.sessions {
		ids = append(ids, id)
	}
	a.sessMu.Unlock()
	for _, id := range ids {
		a.killSession(id, false)
	}
}
