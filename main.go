package main

import (
	"embed"
	"log"

	"github.com/spf13/cobra"
	"github.com/KNSWebWorks/kns-ssh/cmd"
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

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
