//go:build windows

package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"

	"eve-assistant/desktop-app/internal/storage"
	_ "modernc.org/sqlite"
)

// newPersistentDependencies opens the per-user desktop database and configures
// the Windows DPAPI-backed credential store. The returned close function owns
// the database lifecycle.
func newPersistentDependencies() (storage.Store, storage.SecretStore, func() error, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, nil, nil, err
	}
	dataDir := filepath.Join(configDir, "Alice-EVE")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, nil, nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "desktop.sqlite"))
	if err != nil {
		return nil, nil, nil, err
	}
	store := storage.NewSQLiteStore(db)
	if err := storage.EnsureReliableSchema(context.Background(), store); err != nil {
		_ = db.Close()
		return nil, nil, nil, err
	}
	secrets := storage.NewDPAPISecretStore(filepath.Join(dataDir, "secrets"))
	return store, secrets, db.Close, nil
}
