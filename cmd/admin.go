package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

// bootstrapApp returns a bootstrapped PocketBase instance (no HTTP server)
// that uses the same data dir as `serve`.
func bootstrapApp(dataDir string) core.App {
	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: dataDir,
	})
	if err := app.Bootstrap(); err != nil {
		log.Fatalf("Bootstrap failed: %v", err)
	}
	if err := EnsureCollections(app); err != nil {
		log.Fatalf("EnsureCollections failed: %v", err)
	}
	return app
}

func CreateUserCmd() *cobra.Command {
	var email, password, dataDir string

	cmd := &cobra.Command{
		Use:   "create-user",
		Short: "Create a web user (login for the site)",
		Run: func(cmd *cobra.Command, args []string) {
			if email == "" || password == "" {
				log.Fatal("Both --email and --password are required")
			}
			app := bootstrapApp(dataDir)

			user, err := createUser(app, email, password)
			if err != nil {
				log.Fatal(err)
			}
			if user == nil {
				fmt.Printf("User %q already exists\n", email)
				return
			}
			fmt.Printf("Created user %q (id %s)\n", email, user.Id)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "User email (login)")
	cmd.Flags().StringVar(&password, "password", "", "User password")
	cmd.Flags().StringVar(&dataDir, "dir", "pb_data", "PocketBase data directory")
	return cmd
}

func CreateAgentCmd() *cobra.Command {
	var email, name, token, dataDir string

	cmd := &cobra.Command{
		Use:   "create-agent",
		Short: "Register an agent (computer) for a user",
		Run: func(cmd *cobra.Command, args []string) {
			if email == "" || name == "" {
				log.Fatal("Both --email and --name are required")
			}
			if token == "" {
				token = randomToken()
				fmt.Printf("Generated token: %s\n", token)
			}
			app := bootstrapApp(dataDir)

			user, err := findUserByEmail(app, email)
			if err != nil {
				log.Fatalf("User %q not found (create one with create-user)", email)
			}

			rec, err := createAgent(app, user.Id, name, token)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("Created agent %q (id %s) for %q\n", name, rec.Id, email)
			fmt.Printf("Start it with: ./kns-ssh agent -t %s -s ws://SERVER:8090\n", token)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "Owner user email")
	cmd.Flags().StringVar(&name, "name", "", "Agent display name (e.g. my-mac)")
	cmd.Flags().StringVar(&token, "token", "", "Agent token (generated if empty)")
	cmd.Flags().StringVar(&dataDir, "dir", "pb_data", "PocketBase data directory")
	return cmd
}

func randomToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return hex.EncodeToString(b)
}
