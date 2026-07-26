package cmd

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func ServeCmd(distDir embed.FS) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the Central Proxy Server (PocketBase + WS Hub)",
		Run: func(cmd *cobra.Command, args []string) {
			app := pocketbase.New()
			hub := NewHub()

			app.OnServe().BindFunc(func(e *core.ServeEvent) error {
				subFS, err := fs.Sub(distDir, "frontend/dist")
				if err == nil {
					e.Router.GET("/{path...}", apis.Static(subFS, true))
				}

				// Endpoint for Agent to connect
				e.Router.GET("/api/ws/agent", func(req *core.RequestEvent) error {
					agentID := req.Request.URL.Query().Get("token")
					if agentID == "" {
						return req.BadRequestError("Missing agent token", nil)
					}

					conn, err := upgrader.Upgrade(req.Response, req.Request, nil)
					if err != nil {
						log.Println("WS upgrade failed:", err)
						return nil
					}

					hub.RegisterAgent(agentID, conn)
					defer hub.UnregisterAgent(agentID)

					for {
						_, message, err := conn.ReadMessage()
						if err != nil {
							break
						}
						var msg WsMessage
						if err := json.Unmarshal(message, &msg); err == nil {
							hub.RouteToClient(msg.SessionID, msg)
						}
					}
					return nil
				})

				// Endpoint for Web Client to connect
				e.Router.GET("/api/ws/client", func(req *core.RequestEvent) error {
					sessionID := req.Request.URL.Query().Get("session_id")
					agentID := req.Request.URL.Query().Get("agent_id")
					if sessionID == "" || agentID == "" {
						return req.BadRequestError("Missing parameters", nil)
					}

					conn, err := upgrader.Upgrade(req.Response, req.Request, nil)
					if err != nil {
						log.Println("WS upgrade failed:", err)
						return nil
					}

					hub.RegisterClient(sessionID, agentID, conn)
					defer hub.UnregisterClient(sessionID)

					hub.RouteToAgent(sessionID, WsMessage{
						Type:      "start_session",
						SessionID: sessionID,
					})

					for {
						_, message, err := conn.ReadMessage()
						if err != nil {
							break
						}
						var msg WsMessage
						if err := json.Unmarshal(message, &msg); err == nil {
							msg.SessionID = sessionID
							hub.RouteToAgent(sessionID, msg)
						}
					}
					return nil
				})

				return e.Next()
			})

			if err := app.Start(); err != nil {
				log.Fatal(err)
			}
		},
	}
}
