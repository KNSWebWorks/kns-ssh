package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

//go:embed all:frontend/dist
var distDir embed.FS

type ExecuteRequest struct {
	ServerID string `json:"server_id"`
	ToolID   string `json:"tool_id"`
}

func main() {
	app := pocketbase.New()

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Serve embedded frontend
		subFS, err := fs.Sub(distDir, "frontend/dist")
		if err != nil {
			return err
		}

		e.Router.GET("/{path...}", apis.Static(subFS, true))

		// Custom API endpoint for SSH execution
		e.Router.POST("/api/ssh/execute", func(req *core.RequestEvent) error {
			// Require auth
			if req.Auth == nil {
				return req.UnauthorizedError("Authentication required", nil)
			}

			var payload ExecuteRequest
			if err := req.BindBody(&payload); err != nil {
				return req.BadRequestError("Invalid request payload", err)
			}

			// Get server from DB
			serverRecord, err := app.FindRecordById("servers", payload.ServerID)
			if err != nil {
				return req.NotFoundError("Server not found", err)
			}

			// Get tool from DB
			toolRecord, err := app.FindRecordById("tools", payload.ToolID)
			if err != nil {
				return req.NotFoundError("Tool not found", err)
			}

			host := serverRecord.GetString("host")
			port := serverRecord.GetInt("port")
			if port == 0 {
				port = 22
			}
			user := serverRecord.GetString("username")
			authType := serverRecord.GetString("auth_type")
			password := serverRecord.GetString("password")
			privateKey := serverRecord.GetString("private_key")
			command := toolRecord.GetString("command")

			// Execute command
			result, sshErr := ExecuteSSHCommand(host, port, user, authType, password, privateKey, command)
			
			if sshErr != nil {
				return req.JSON(http.StatusInternalServerError, map[string]interface{}{
					"error": sshErr.Error(),
					"details": result,
				})
			}

			return req.JSON(http.StatusOK, result)
		})

		return e.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
