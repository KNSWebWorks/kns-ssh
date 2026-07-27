//go:build !darwin && !windows

package cmd

import (
	"log"

	"github.com/spf13/cobra"
)

func AgentCmd() *cobra.Command {
	var token, server string
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Start the KNS SSH Agent (Headless)",
		Run: func(cmd *cobra.Command, args []string) {
			if token == "" || server == "" {
				log.Fatal("Both --token and --server are required")
			}

			agent := NewAgent(token, server)
			agent.setStatus = func(status string) {
				log.Println("Status:", status)
			}
			agent.RunLoop()
		},
	}
	cmd.Flags().StringVarP(&token, "token", "t", "", "Agent Authentication Token")
	cmd.Flags().StringVarP(&server, "server", "s", "", "WebSocket Server URL (e.g. wss://app.railway.app)")
	return cmd
}
