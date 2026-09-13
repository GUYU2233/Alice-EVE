package storage

import "errors"

var ErrSecretNotFound = errors.New("secret not found")

// SecretStore abstracts platform credential vaults; implementations must avoid logging values.
type SecretStore interface {
	Save(key, value string) error
	Load(key string) (string, error)
	Delete(key string) error
}

// MemorySecretStore is intended for tests and development only.
type MemorySecretStore struct{ values map[string]string }
func NewMemorySecretStore() *MemorySecretStore { return &MemorySecretStore{values: map[string]string{}} }
func (s *MemorySecretStore) Save(k,v string) error { if k=="" { return errors.New("secret key required") }; s.values[k]=v; return nil }
func (s *MemorySecretStore) Load(k string) (string,error) { v,ok:=s.values[k]; if !ok { return "",ErrSecretNotFound }; return v,nil }
func (s *MemorySecretStore) Delete(k string) error { delete(s.values,k); return nil }
