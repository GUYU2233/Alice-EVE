package storage

import (
	"errors"
	"sync"
)

var (
	ErrSecretNotFound = errors.New("secret not found")
	// ErrSecureStorageUnavailable is returned by non-Windows defaults. A token
	// must never silently fall back to plaintext or process memory in production.
	ErrSecureStorageUnavailable = errors.New("secure storage is unavailable")
)

// SecretStore abstracts platform credential vaults; implementations must avoid logging values.
type SecretStore interface {
	Save(key, value string) error
	Load(key string) (string, error)
	Delete(key string) error
}

// SecureStorageAdapter is the narrow adapter implemented by OS vault bindings.
// macOS Keychain and Linux Secret Service adapters can be supplied by the host
// application without adding a platform dependency to the core package.
type SecureStorageAdapter interface{ SecretStore }

type adapterSecretStore struct {
	adapter SecureStorageAdapter
	name    string
}

func (s *adapterSecretStore) Save(k, v string) error {
	if s.adapter == nil {
		return ErrSecureStorageUnavailable
	}
	return s.adapter.Save(k, v)
}
func (s *adapterSecretStore) Load(k string) (string, error) {
	if s.adapter == nil {
		return "", ErrSecureStorageUnavailable
	}
	return s.adapter.Load(k)
}
func (s *adapterSecretStore) Delete(k string) error {
	if s.adapter == nil {
		return nil
	}
	return s.adapter.Delete(k)
}

// NewKeychainSecretStore and NewSecretServiceSecretStore deliberately accept
// injected adapters. This keeps OS integration pluggable and testable.
func NewKeychainSecretStore(adapter SecureStorageAdapter) SecretStore {
	return &adapterSecretStore{adapter: adapter, name: "macOS Keychain"}
}
func NewSecretServiceSecretStore(adapter SecureStorageAdapter) SecretStore {
	return &adapterSecretStore{adapter: adapter, name: "Linux Secret Service"}
}

// FailClosedSecretStore is the safe default when no OS vault adapter is linked.
type FailClosedSecretStore struct{}

func NewFailClosedSecretStore() SecretStore               { return FailClosedSecretStore{} }
func (FailClosedSecretStore) Save(string, string) error   { return ErrSecureStorageUnavailable }
func (FailClosedSecretStore) Load(string) (string, error) { return "", ErrSecureStorageUnavailable }
func (FailClosedSecretStore) Delete(string) error         { return nil }

// MemorySecretStore is intended for tests only.
type MemorySecretStore struct {
	mu     sync.RWMutex
	values map[string]string
}

func NewMemorySecretStore() *MemorySecretStore {
	return &MemorySecretStore{values: map[string]string{}}
}
func (s *MemorySecretStore) Save(k, v string) error {
	if k == "" {
		return errors.New("secret key required")
	}
	s.mu.Lock()
	s.values[k] = v
	s.mu.Unlock()
	return nil
}
func (s *MemorySecretStore) Load(k string) (string, error) {
	s.mu.RLock()
	v, ok := s.values[k]
	s.mu.RUnlock()
	if !ok {
		return "", ErrSecretNotFound
	}
	return v, nil
}
func (s *MemorySecretStore) Delete(k string) error {
	s.mu.Lock()
	delete(s.values, k)
	s.mu.Unlock()
	return nil
}
