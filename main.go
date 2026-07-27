package main

import (
	"embed"
	"log"

	"github.com/KNSWebWorks/kns-ssh/cmd"
	"github.com/spf13/cobra"
)

//go:embed all:frontend/dist
var distDir embed.FS

func main() {
	rootCmd := &cobra.Command{
		Use:   "kns-ssh",
		Short: "KNS SSH Proxy and Agent",
	}

	rootCmd.AddCommand(cmd.ServeCmd(distDir))
	rootCmd.AddCommand(cmd.AgentCmd())
	rootCmd.AddCommand(cmd.CreateUserCmd())
	rootCmd.AddCommand(cmd.CreateAgentCmd())

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
