package eve

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const BundledSDEVersion = "3503375"

//go:embed assets/eve-sde-3503375.sqlite.gz
var bundledSDE []byte

// EnsureBundledSDE atomically installs the bundled, Chinese-first official CCP
// SDE when the user's cached database is absent or older than this build.
func EnsureBundledSDE(ctx context.Context, target string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	idx := NewSDEIndex("")
	if st, err := idx.OpenDatabase(ctx, target); err == nil && st.Version >= BundledSDEVersion {
		_ = idx.Close()
		return nil
	}
	_ = idx.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".eve-sde-*.sqlite")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	gz, err := gzip.NewReader(bytes.NewReader(bundledSDE))
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("open bundled SDE: %w", err)
	}
	_, copyErr := io.Copy(tmp, &contextReader{ctx: ctx, r: gz})
	closeGZ := gz.Close()
	closeFile := tmp.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeGZ != nil {
		return closeGZ
	}
	if closeFile != nil {
		return closeFile
	}
	if err = os.Rename(tmpName, target); err != nil {
		_ = os.Remove(target)
		if err = os.Rename(tmpName, target); err != nil {
			return err
		}
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
