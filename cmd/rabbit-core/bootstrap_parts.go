package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var rabbitBootstrapPartsBaseURL string

var rabbitBootstrapPartHashes = [...]string{
	"430adeda090d1c86fdbf7069456265a6ea8f043052dff010d84b9a3a7dacb2ac",
	"3a39e31443d1045f8f6e962bbed7d25fbd4a9338b13c99b30ddfa88ee717e97c",
	"9d89ac83b77595ef4d546499fc88b18ee0f102ceef9bee92bbc6d917423cb0cd",
	"273840520288096bd0a4ac3478fe156d65ad17080ed63607b26bbc1e9adaa51a",
}

func bootstrapMultipartFrom(ctx context.Context, dataDir, baseURL string) error {
	if _, err := os.Lstat(filepath.Join(dataDir, "rabbit", "chaindata")); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Lstat(filepath.Join(dataDir, "rabbit")); err == nil {
		return errors.New("rabbit directory already exists; refusing to overwrite")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !strings.HasPrefix(baseURL, "https://") {
		return errors.New("bootstrap parts require HTTPS")
	}

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return err
	}

	client := &http.Client{}
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if r.URL.Scheme != "https" || len(via) >= 5 {
			return errors.New("unsafe bootstrap redirect")
		}
		return nil
	}

	archive := filepath.Join(dataDir, ".rabbit-bootstrap-"+rabbitBootstrapSHA256+".part")
	out, err := os.OpenFile(archive, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	hash := sha256.New()

	for i, expected := range rabbitBootstrapPartHashes {
		name := fmt.Sprintf("rabbit-testnet-bootstrap-v2411.part-%03d", i)
		part := filepath.Join(dataDir, ".rabbit-bootstrap-"+name)

		if err := downloadPartIfNeeded(ctx, client,
			strings.TrimRight(baseURL, "/")+"/"+name, part, expected); err != nil {
			out.Close()
			return err
		}

		f, err := os.Open(part)
		if err != nil {
			out.Close()
			return err
		}

		_, copyErr := io.Copy(io.MultiWriter(out, hash), f)
		closeErr := f.Close()
		if copyErr != nil || closeErr != nil {
			out.Close()
			return errors.New("failed assembling bootstrap parts")
		}
	}

	syncErr := out.Sync()
	closeErr := out.Close()
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}

	if hex.EncodeToString(hash.Sum(nil)) != rabbitBootstrapSHA256 {
		return errors.New("assembled bootstrap SHA-256 mismatch")
	}
	return installVerifiedBootstrap(dataDir, archive)
}

func downloadPartIfNeeded(ctx context.Context, client *http.Client, url, path, expected string) error {
	valid, err := bootstrapFileMatches(path, expected)
	if err != nil {
		return err
	}
	if valid {
		return nil
	}
	return downloadResumable(ctx, client, url, path, expected)
}

func bootstrapFileMatches(path, expected string) (bool, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return false, err
	}
	if !st.Mode().IsRegular() || st.Size() > maxBootstrapDownload {
		return false, errors.New("invalid bootstrap part")
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return hex.EncodeToString(h.Sum(nil)) == expected, nil
}
