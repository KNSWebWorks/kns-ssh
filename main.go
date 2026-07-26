package main

import (
	"embed"
	"log"

	"github.com/spf13/cobra"
	"github.com/user/ssh-assistant/cmd"
)

//go:embed all:frontend/dist
var distDir embed.FS

func main() {
	rootCmd := &cobra.Command{
		Use:   "ssh-assistant",
		Short: "SSH Assistant Proxy and Agent",
	}

	rootCmd.AddCommand(cmd.ServeCmd(distDir))
	rootCmd.AddCommand(cmd.AgentCmd())

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
