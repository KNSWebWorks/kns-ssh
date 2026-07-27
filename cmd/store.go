package cmd

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// EnsureCollections creates the "agents" collection on first run.
// Fields: name (text), token (text, unique), user (relation -> users).
func EnsureCollections(app core.App) error {
	if _, err := app.FindCollectionByNameOrId("agents"); err == nil {
		return nil // already exists
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return fmt.Errorf("users collection not found: %w", err)
	}

	col := core.NewBaseCollection("agents")
	col.Fields.Add(
		&core.TextField{Name: "name", Required: true},
		&core.TextField{Name: "token", Required: true},
		&core.RelationField{Name: "user", CollectionId: users.Id, Required: true, MaxSelect: 1},
	)
	// Unique token per agent.
	col.Indexes = types.JSONArray[string]{
		"CREATE UNIQUE INDEX idx_agents_token ON agents (token)",
	}
	// Owners can see only their own agents; create/update/delete stay admin-only.
	col.ListRule = types.Pointer("user = @request.auth.id")
	col.ViewRule = types.Pointer("user = @request.auth.id")

	if err := app.Save(col); err != nil {
		return fmt.Errorf("failed to create agents collection: %w", err)
	}
	return nil
}

// createUser creates a web user; returns (nil, nil) if the user already exists.
func createUser(app core.App, email, password string) (*core.Record, error) {
	if existing, _ := findUserByEmail(app, email); existing != nil {
		return nil, nil
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return nil, err
	}

	rec := core.NewRecord(users)
	rec.Set("email", email)
	rec.SetPassword(password)
	if err := app.Save(rec); err != nil {
		return nil, err
	}
	return rec, nil
}

func findUserByEmail(app core.App, email string) (*core.Record, error) {
	return app.FindFirstRecordByFilter("users", "email = {:email}", dbx.Params{"email": email})
}

// createAgent registers an agent (name + token) for the given user id.
func createAgent(app core.App, userID, name, token string) (*core.Record, error) {
	agents, err := app.FindCollectionByNameOrId("agents")
	if err != nil {
		return nil, err
	}

	rec := core.NewRecord(agents)
	rec.Set("name", name)
	rec.Set("token", token)
	rec.Set("user", userID)
	if err := app.Save(rec); err != nil {
		return nil, err
	}
	return rec, nil
}
