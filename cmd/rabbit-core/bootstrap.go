package main

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// Set at release build time using -ldflags '-X main.rabbitBootstrapURL=https://...'.
// Never publish a release with an unverified URL. The URL identifies immutable bytes.
var rabbitBootstrapURL string

const rabbitBootstrapSHA256 = "a3d79f8924bd019cac2d7d8ee1c3674a1db2b7b5e176a227a16b26c5d5193825"
const maxBootstrapDownload int64 = 16 << 30
const maxBootstrapExpanded int64 = 40 << 30

func bootstrapFreshNode(ctx context.Context, dataDir string) error {
	if rabbitBootstrapPartsBaseURL != "" {
		return bootstrapMultipartFrom(ctx, dataDir, rabbitBootstrapPartsBaseURL)
	}
	if rabbitBootstrapURL == "" {
		return nil
	} // Old full-sync mode; release gating must forbid this for bootstrap releases.
	return bootstrapFrom(ctx, dataDir, rabbitBootstrapURL, rabbitBootstrapSHA256, nil)
}

// bootstrapFrom is separated for end-to-end tests. It never changes existing databases.
func bootstrapFrom(ctx context.Context, dataDir, sourceURL, expectedSHA string, client *http.Client) error {
	chain := filepath.Join(dataDir, "rabbit", "chaindata")
	if _, err := os.Lstat(chain); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect chaindata: %w", err)
	}
	// Never replace the Rabbit directory: it could hold keystore, nodekey or other state.
	rabbitDir := filepath.Join(dataDir, "rabbit")
	if _, err := os.Lstat(rabbitDir); err == nil {
		return fmt.Errorf("rabbit directory already exists without chaindata; refusing to overwrite: %s", rabbitDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !strings.HasPrefix(sourceURL, "https://") {
		return errors.New("bootstrap source must be HTTPS")
	}
	if len(expectedSHA) != 64 {
		return errors.New("invalid expected bootstrap SHA-256")
	}
	if _, err := hex.DecodeString(expectedSHA); err != nil {
		return fmt.Errorf("invalid expected SHA-256: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return err
	}
	if client == nil {
		client = &http.Client{Timeout: 0}
	}
	// Redirect validation applies also to a caller-supplied HTTP client.
	safeClient := *client
	safeClient.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if r.URL.Scheme != "https" {
			return errors.New("bootstrap redirects must use HTTPS")
		}
		if len(via) >= 5 {
			return errors.New("too many bootstrap redirects")
		}
		return nil
	}
	partial := filepath.Join(dataDir, ".rabbit-bootstrap-"+expectedSHA+".part")
	if err := downloadResumable(ctx, &safeClient, sourceURL, partial, expectedSHA); err != nil {
		return err
	}
	return installVerifiedBootstrap(dataDir, partial)
}

func installVerifiedBootstrap(dataDir, partial string) error {
	rabbitDir := filepath.Join(dataDir, "rabbit")
	// Extraction occurs entirely in a private staging directory on the target filesystem.
	work, err := os.MkdirTemp(dataDir, ".rabbit-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	if err := extractRabbitArchive(partial, work); err != nil {
		return err
	}
	staged := filepath.Join(work, "rabbit")
	for _, name := range []string{"chaindata", "triedb"} {
		fi, e := os.Lstat(filepath.Join(staged, name))
		if e != nil {
			return fmt.Errorf("bootstrap missing %s: %w", name, e)
		}
		if !fi.IsDir() {
			return fmt.Errorf("bootstrap %s is not a directory", name)
		}
	}
	// Rename is atomic on the same filesystem. It cannot overwrite existing rabbit data.
	if _, err := os.Lstat(rabbitDir); err == nil {
		return errors.New("rabbit data appeared during download; refusing install")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(staged, rabbitDir); err != nil {
		return fmt.Errorf("commit verified bootstrap: %w", err)
	}
	_ = os.Remove(partial)
	return nil
}

func downloadResumable(ctx context.Context, client *http.Client, source, partial, expected string) error {
	var offset int64
	if st, err := os.Lstat(partial); err == nil {
		if !st.Mode().IsRegular() {
			return errors.New("bootstrap partial path is not a regular file")
		}
		offset = st.Size()
		if offset > maxBootstrapDownload {
			return errors.New("bootstrap partial exceeds limit")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		req.Header.Set("Accept-Encoding", "identity")
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("bootstrap transfer: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		offset = 0 // Origin ignored Range: start over, never append duplicate bytes.
	case http.StatusPartialContent:
		if offset == 0 {
			return errors.New("unexpected partial content")
		}
		cr := resp.Header.Get("Content-Range")
		if !strings.HasPrefix(cr, fmt.Sprintf("bytes %d-", offset)) {
			return fmt.Errorf("invalid Content-Range %q", cr)
		}
		// Parse total to reject bogus or oversized archives.
		parts := strings.Split(cr, "/")
		if len(parts) != 2 {
			return errors.New("invalid Content-Range total")
		}
		total, e := strconv.ParseInt(parts[1], 10, 64)
		if e != nil || total < offset || total > maxBootstrapDownload {
			return errors.New("invalid archive length in Content-Range")
		}
	default:
		return fmt.Errorf("bootstrap HTTP status %d", resp.StatusCode)
	}
	flag := os.O_WRONLY | os.O_CREATE
	if offset == 0 {
		flag |= os.O_TRUNC
	} else {
		flag |= os.O_APPEND
	}
	f, err := os.OpenFile(partial, flag, 0600)
	if err != nil {
		return err
	}
	written, copyErr := copyBootstrapWithProgress(f, io.LimitReader(resp.Body, maxBootstrapDownload-offset+1), offset, resp.ContentLength)
	syncErr := f.Sync()
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("download interrupted after %d bytes: %w", written, copyErr)
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written+offset > maxBootstrapDownload {
		return errors.New("bootstrap archive exceeds size limit")
	}
	// Catch truncated bodies when server advertises Content-Length.
	if resp.ContentLength >= 0 && written != resp.ContentLength {
		return errors.New("bootstrap body incomplete")
	}
	file, err := os.Open(partial)
	if err != nil {
		return err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(sum.Sum(nil))
	if actual != expected {
		return fmt.Errorf("bootstrap SHA-256 mismatch: got %s", actual)
	}
	return nil
}

func extractRabbitArchive(archivePath, work string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := zstd.NewReader(f)
	if err != nil {
		return err
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	base := filepath.Join(work, "rabbit")
	var expanded int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		// Reject backslashes on all platforms, dot entries, absolute paths and drive prefixes.
		if strings.Contains(h.Name, "\\") || filepath.IsAbs(h.Name) || strings.Contains(h.Name, ":") {
			return fmt.Errorf("unsafe archive path %q", h.Name)
		}
		clean := filepath.Clean(filepath.FromSlash(h.Name))
		if clean != "rabbit" && !strings.HasPrefix(clean, "rabbit"+string(filepath.Separator)) {
			return fmt.Errorf("unsafe archive entry %q", h.Name)
		}
		valid := clean == "rabbit" || clean == filepath.Join("rabbit", "chaindata") || clean == filepath.Join("rabbit", "triedb") || strings.HasPrefix(clean, filepath.Join("rabbit", "chaindata")+string(filepath.Separator)) || strings.HasPrefix(clean, filepath.Join("rabbit", "triedb")+string(filepath.Separator))
		if !valid {
			return fmt.Errorf("unexpected archive entry %q", h.Name)
		}
		path := filepath.Join(work, clean)
		if path != base && !strings.HasPrefix(path, base+string(filepath.Separator)) {
			return fmt.Errorf("path escape %q", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if h.Size < 0 || h.Size > maxBootstrapExpanded-expanded {
				return errors.New("extraction exceeds safe limit")
			}
			expanded += h.Size
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
			out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(out, tr, h.Size)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("unsupported archive entry %q", h.Name)
		}
	}
	return nil
}

// copyBootstrapWithProgress reports transfer progress without changing the
// existing download limits, resume offsets or SHA-256 verification.
func copyBootstrapWithProgress(dst io.Writer, src io.Reader, offset, remaining int64) (int64, error) {
	const reportEvery int64 = 32 << 20
	const mib int64 = 1 << 20

	var written int64
	nextReport := reportEvery
	total := int64(-1)
	if remaining >= 0 {
		total = offset + remaining
	}

	fmt.Printf("Rabbit Bootstrap: download started (resuming at %d MiB)\\n", offset/mib)

	buf := make([]byte, 1024*1024)
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			nw, writeErr := dst.Write(buf[:n])
			written += int64(nw)
			if writeErr != nil {
				return written, writeErr
			}
			if nw != n {
				return written, io.ErrShortWrite
			}

			if written >= nextReport {
				if total > 0 {
					fmt.Printf("Rabbit Bootstrap: %d / %d MiB downloaded\\n", (offset+written)/mib, total/mib)
				} else {
					fmt.Printf("Rabbit Bootstrap: %d MiB downloaded\\n", (offset+written)/mib)
				}
				nextReport = written + reportEvery
			}
		}
		if readErr == io.EOF {
			fmt.Printf("Rabbit Bootstrap: transfer finished (%d MiB received)\\n", (offset+written)/mib)
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
		if n == 0 {
			return written, io.ErrNoProgress
		}
	}
}
