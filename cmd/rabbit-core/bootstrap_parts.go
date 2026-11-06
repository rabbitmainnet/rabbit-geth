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
	"cda78f76a3e834bc108403597d5123f41975486d083adcdb7cd357e983b0500c",
	"5dd4334127dedc5e43f255d388f75e8578e897c4206beac75767ef77960ae5ba",
	"41d8766a07c54a3913d0a74ae060a02b7b2437cad664ec74a490ef043946e92d",
	"498abc86ba3bb2c10aeff681c5656255dd9d6932e9268248838a8e96209d624a",
	"cf3d9be982c0bbe9ecc0aae1979bac820592fe4cbd01c5c41758f7ed9c3676cc",
	"f9979b44bdb53e87750838b7959b7389b2af6d5e7f8fa2a906f8efb2b80594ee",
	"ad6229db7d95267ac269ae53ebb2d167f798fd451e05b012175c3712cee53c9a",
	"3d02a3b5d7b1ee8e9511d35a5c751f0295cc39b5b28170937d20d7ecd78c51b0",
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

	fmt.Println("Rabbit Bootstrap: preparing verified blockchain snapshot...")
	for i, expected := range rabbitBootstrapPartHashes {
		fmt.Printf("Rabbit Bootstrap: checking/downloading part %d of %d...\n", i+1, len(rabbitBootstrapPartHashes))
		name := fmt.Sprintf("rabbit-testnet-bootstrap-385509.part-%03d", i)
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
