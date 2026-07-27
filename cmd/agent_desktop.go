//go:build darwin || windows

package cmd

import (
	"log"

	"github.com/getlantern/systray"
	"github.com/spf13/cobra"
)

func AgentCmd() *cobra.Command {
	var token, server string
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Start the KNS SSH Agent",
		Run: func(cmd *cobra.Command, args []string) {
			if token == "" || server == "" {
				log.Fatal("Both --token and --server are required")
			}

			agent := NewAgent(token, server)

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
				// Trigger a reconnect by closing the current connection
				a.closeConn()
			}
		}
	}()

	// Background Agent Logic
	go a.RunLoop()
}

func (a *Agent) onExit() {
	a.closeConn()
	a.closeAllSessions()
}
