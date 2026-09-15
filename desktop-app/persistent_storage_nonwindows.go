//go:build !windows

package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"

	"eve-assistant/desktop-app/internal/storage"
	_ "modernc.org/sqlite"
)

// newPersistentDependencies provides the same per-user persistence contract
// on development platforms. Credentials require an injected Keychain/Secret
// Service adapter; absent one, the default is fail-closed (never plaintext).
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
	st := storage.NewSQLiteStore(db)
	if err := storage.EnsureReliableSchema(context.Background(), st); err != nil {
		_ = db.Close()
		return nil, nil, nil, err
	}
	return st, storage.NewFailClosedSecretStore(), db.Close, nil
}
