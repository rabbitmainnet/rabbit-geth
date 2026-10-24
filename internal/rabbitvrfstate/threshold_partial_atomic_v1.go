package rabbitvrfstate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Used only for public threshold partial packets.
// A successful return follows file Sync, no-replace installation and directory Sync.
func atomicWriteThresholdPartialFileV1(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrInvalidDKGTransportKeyStoreV1
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return err
	}
	if err := requireTransportKeyPathAbsentV1(path); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer func() {
		temp.Close()
		os.Remove(name)
	}()
	if err := temp.Chmod(0600); err != nil {
		return err
	}
	written, err := temp.Write(content)
	if err != nil {
		return err
	}
	if written != len(content) {
		return io.ErrShortWrite
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync threshold partial file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := requireTransportKeyPathAbsentV1(path); err != nil {
		return err
	}
	if err := os.Link(name, path); err != nil {
		if os.IsExist(err) {
			return ErrDKGTransportKeyStoreAlreadyExistsV1
		}
		return err
	}
	if err := os.Remove(name); err != nil {
		return err
	}
	if err := syncDirectoryV1(dir); err != nil {
		return fmt.Errorf("sync threshold partial directory: %w", err)
	}
	return nil
}
