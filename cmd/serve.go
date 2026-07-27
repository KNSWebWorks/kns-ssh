package cmd

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func ServeCmd(distDir embed.FS) *cobra.Command {
	var httpAddr, dataDir string

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Central Proxy Server (PocketBase + WS Hub)",
		Run: func(cmd *cobra.Command, args []string) {
			app := pocketbase.New()
			hub := NewHub()

			app.OnServe().BindFunc(func(e *core.ServeEvent) error {
				if err := EnsureCollections(e.App); err != nil {
					return err
				}
				if err := bootstrapFromEnv(e.App); err != nil {
					log.Println("env bootstrap:", err)
				}

				subFS, err := fs.Sub(distDir, "frontend/dist")
				if err == nil {
					e.Router.GET("/{path...}", apis.Static(subFS, true))
				}

				// List the current user's agents with online status.
				e.Router.GET("/api/agents/online", func(req *core.RequestEvent) error {
					records, err := e.App.FindRecordsByFilter(
						"agents",
						"user = {:userID}",
						"",
						100,
						0,
						dbx.Params{"userID": req.Auth.Id},
					)
					if err != nil {
						log.Println("list agents error:", err)
						return req.InternalServerError("Failed to list agents", err)
					}

					type agentInfo struct {
						Name   string `json:"name"`
						Token  string `json:"token"`
						Online bool   `json:"online"`
					}
					result := make([]agentInfo, 0, len(records))
					for _, rec := range records {
						token := rec.GetString("token")
						result = append(result, agentInfo{
							Name:   rec.GetString("name"),
							Token:  token,
							Online: hub.IsOnline(token),
						})
					}
					return req.JSON(http.StatusOK, result)
				}).Bind(apis.RequireAuth("users"))

				// Endpoint for Agent to connect
				e.Router.GET("/api/ws/agent", func(req *core.RequestEvent) error {
					token := req.Request.URL.Query().Get("token")
					if token == "" {
						return req.BadRequestError("Missing agent token", nil)
					}

					// Validate the token against the agents collection.
					rec, err := e.App.FindFirstRecordByFilter(
						"agents",
						"token = {:token}",
						dbx.Params{"token": token},
					)
					if err != nil {
						return req.UnauthorizedError("Unknown agent token", nil)
					}

					conn, err := upgrader.Upgrade(req.Response, req.Request, nil)
					if err != nil {
						log.Println("WS upgrade failed:", err)
						return nil
					}

					hub.RegisterAgent(token, rec.GetString("name"), rec.GetString("user"), conn)
					defer hub.UnregisterAgent(token)

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
					if !hub.IsOnline(agentID) {
						return req.BadRequestError("Agent is offline", nil)
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

			app.RootCmd.SetArgs([]string{"serve", "--http=" + httpAddr, "--dir=" + dataDir})

			if err := app.Start(); err != nil {
				log.Fatal(err)
			}
		},
	}

	serveCmd.Flags().StringVar(&httpAddr, "http", "127.0.0.1:8090", "HTTP server address (host:port)")
	serveCmd.Flags().StringVar(&dataDir, "dir", "pb_data", "PocketBase data directory")

	return serveCmd
}

// bootstrapFromEnv creates the initial user and agent from environment
// variables (useful on Railway, where there is no shell access):
//
//	KNS_ADMIN_EMAIL, KNS_ADMIN_PASSWORD — first web user
//	KNS_AGENT_NAME, KNS_AGENT_TOKEN     — first agent of that user
func bootstrapFromEnv(app core.App) error {
	email := os.Getenv("KNS_ADMIN_EMAIL")
	password := os.Getenv("KNS_ADMIN_PASSWORD")
	if email == "" || password == "" {
		return nil
	}

	user, err := createUser(app, email, password)
	if err != nil {
		return err
	}
	if user == nil {
		return nil // user already existed
	}
	log.Printf("Created initial user %q from env", email)

	agentName := os.Getenv("KNS_AGENT_NAME")
	agentToken := os.Getenv("KNS_AGENT_TOKEN")
	if agentName != "" && agentToken != "" {
		if _, err := createAgent(app, user.Id, agentName, agentToken); err != nil {
			return err
		}
		log.Printf("Created initial agent %q from env", agentName)
	}
	return nil
}
