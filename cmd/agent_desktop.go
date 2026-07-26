//go:build darwin || windows

package cmd

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/getlantern/systray"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

type Agent struct {
	Token      string
	Server     string
	Conn       *websocket.Conn
	Sessions   map[string]*os.File
	mu         sync.Mutex
	reconnect  chan struct{}
	setStatus  func(string)
}

func AgentCmd() *cobra.Command {
	var token, server string
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Start the KNS SSH Agent",
		Run: func(cmd *cobra.Command, args []string) {
			if token == "" || server == "" {
				log.Fatal("Both --token and --server are required")
			}
			
			agent := &Agent{
				Token:     token,
				Server:    server,
				Sessions:  make(map[string]*os.File),
				reconnect: make(chan struct{}, 1),
			}
			
			// systray.Run blocks the main thread (required by macOS)
			systray.Run(agent.onReady, agent.onExit)
		},
	}
	cmd.Flags().StringVarP(&token, "token", "t", "", "Agent Authentication Token")
	cmd.Flags().StringVarP(&server, "server", "s", "", "WebSocket Server URL (e.g. wss://app.railway.app)")
	return cmd
}

func (a *Agent) onReady() {
	systray.SetTitle("KNS")
	systray.SetTooltip("KNS SSH Agent")

	mStatus := systray.AddMenuItem("Status: Connecting...", "Connection Status")
	mStatus.Disable()
	
	mReconnect := systray.AddMenuItem("Reconnect", "Force reconnection")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Quit Agent")

	a.setStatus = func(status string) {
		mStatus.SetTitle("Status: " + status)
		log.Println("Status:", status)
	}

	// UI Events
	go func() {
		for {
			select {
			case <-mQuit.ClickedCh:
				systray.Quit()
			case <-mReconnect.ClickedCh:
				// Trigger a reconnect by closing current connection
				a.mu.Lock()
				if a.Conn != nil {
					a.Conn.Close()
				}
				a.mu.Unlock()
			}
		}
	}()

	// Background Agent Logic
	go a.RunLoop()
}

func (a *Agent) onExit() {
	// Cleanup here
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.Conn != nil {
		a.Conn.Close()
	}
	for _, ptmx := range a.Sessions {
		ptmx.Close()
	}
}

func (a *Agent) connect() error {
	url := a.Server + "/api/ws/agent?token=" + a.Token
	a.setStatus("Connecting...")
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.Conn = c
	a.mu.Unlock()
	a.setStatus("Connected")
	return nil
}

func (a *Agent) RunLoop() {
	for {
		err := a.connect()
		if err != nil {
			a.setStatus("Error (Retrying in 5s)")
			time.Sleep(5 * time.Second)
			continue
		}

		for {
			a.mu.Lock()
			conn := a.Conn
			a.mu.Unlock()

			if conn == nil {
				break
			}

			_, message, err := conn.ReadMessage()
			if err != nil {
				a.setStatus("Disconnected")
				conn.Close()
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
	a.mu.Lock()
	defer a.mu.Unlock()

	switch msg.Type {
	case "start_session":
		if _, exists := a.Sessions[msg.SessionID]; !exists {
			log.Printf("Starting new PTY for session %s", msg.SessionID)
			
			// Start bash
			c := exec.Command("bash")
			ptmx, err := pty.Start(c)
			if err != nil {
				log.Printf("Failed to start pty: %v", err)
				return
			}
			a.Sessions[msg.SessionID] = ptmx

			// Read from PTY and send to WS
			go func(sessionID string, p *os.File) {
				buf := make([]byte, 1024)
				for {
					n, err := p.Read(buf)
					if err != nil {
						// PTY closed
						a.mu.Lock()
						delete(a.Sessions, sessionID)
						a.mu.Unlock()
						return
					}
					
					outMsg := WsMessage{
						Type:      "terminal_data",
						SessionID: sessionID,
						Data:      string(buf[:n]),
					}
					b, _ := json.Marshal(outMsg)
					
					a.mu.Lock()
					if a.Conn != nil {
						a.Conn.WriteMessage(websocket.TextMessage, b)
					}
					a.mu.Unlock()
				}
			}(msg.SessionID, ptmx)
		}
	case "terminal_data":
		if ptmx, exists := a.Sessions[msg.SessionID]; exists {
			ptmx.Write([]byte(msg.Data))
		}
	case "resize":
		if ptmx, exists := a.Sessions[msg.SessionID]; exists {
			pty.Setsize(ptmx, &pty.Winsize{
				Rows: uint16(msg.Rows),
				Cols: uint16(msg.Cols),
			})
		}
	}
}
