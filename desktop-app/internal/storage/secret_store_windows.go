//go:build windows

package storage

import (
	"errors"
	"github.com/google/uuid"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"unsafe"
)

// DPAPISecretStore stores secrets encrypted for the current Windows user.
type DPAPISecretStore struct{ dir string }

func NewDPAPISecretStore(dir string) *DPAPISecretStore { return &DPAPISecretStore{dir: dir} }
func (s *DPAPISecretStore) path(k string) string {
	return filepath.Join(s.dir, uuid.NewSHA1(uuid.Nil, []byte(k)).String()+".secret")
}
func (s *DPAPISecretStore) Save(k, v string) error {
	if k == "" {
		return errors.New("secret key required")
	}
	in := []byte(v)
	var out windows.DataBlob
	err := windows.CryptProtectData(&windows.DataBlob{Size: uint32(len(in)), Data: &in[0]}, nil, &out, 0, nil, 0, nil)
	if err != nil {
		return err
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(out.Data))))
	if err = os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	p := s.path(k)
	tmp := p + "." + uuid.NewString()
	if err = os.WriteFile(tmp, unsafe.Slice(out.Data, out.Size), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
func (s *DPAPISecretStore) Load(k string) (string, error) {
	b, err := os.ReadFile(s.path(k))
	if os.IsNotExist(err) {
		return "", ErrSecretNotFound
	}
	if err != nil {
		return "", err
	}
	var out windows.DataBlob
	err = windows.CryptUnprotectData(&windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}, nil, nil, 0, nil, 0, &out)
	if err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(out.Data))))
	return string(unsafe.Slice(out.Data, out.Size)), nil
}
func (s *DPAPISecretStore) Delete(k string) error {
	err := os.Remove(s.path(k))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
