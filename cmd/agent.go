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
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

type Agent struct {
	Token    string
	Server   string
	Conn     *websocket.Conn
	Sessions map[string]*os.File // SessionID -> PTY
	mu       sync.Mutex
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
				Token:    token,
				Server:   server,
				Sessions: make(map[string]*os.File),
			}
			
			agent.RunLoop()
		},
	}
	cmd.Flags().StringVarP(&token, "token", "t", "", "Agent Authentication Token")
	cmd.Flags().StringVarP(&server, "server", "s", "", "WebSocket Server URL (e.g. wss://app.railway.app)")
	return cmd
}

func (a *Agent) connect() error {
	url := a.Server + "/api/ws/agent?token=" + a.Token
	log.Printf("Connecting to %s...", url)
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return err
	}
	a.Conn = c
	log.Println("Connected to server.")
	return nil
}

func (a *Agent) RunLoop() {
	for {
		err := a.connect()
		if err != nil {
			log.Printf("Connection failed: %v. Retrying in 5 seconds...", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for {
			_, message, err := a.Conn.ReadMessage()
			if err != nil {
				log.Println("Disconnected:", err)
				a.Conn.Close()
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
						if err != io.EOF {
							log.Printf("PTY read error: %v", err)
						}
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
