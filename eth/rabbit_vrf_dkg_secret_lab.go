//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"crypto/ecdsa"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

const maxRabbitVRFDKGPasswordFileSizeV1 = 1 << 20

func zeroRabbitVRFDKGBytesV1(data []byte) {
	for i := range data {
		data[i] = 0
	}
}

func zeroRabbitVRFDKGPrivateKeyV1(key *ecdsa.PrivateKey) {
	if key != nil && key.D != nil {
		key.D.SetInt64(0)
	}
}

func readRabbitVRFDKGPasswordFileV1(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf(
			"%w: missing dkg password file",
			errRabbitVRFDKGRuntimeV1,
		)
	}

	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf(
			"%w: inspect dkg password file: %v",
			errRabbitVRFDKGRuntimeV1,
			err,
		)
	}

	if !info.Mode().IsRegular() {
		return "", fmt.Errorf(
			"%w: dkg password file is not regular",
			errRabbitVRFDKGRuntimeV1,
		)
	}

	if runtime.GOOS != "windows" &&
		info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf(
			"%w: unsafe dkg password permissions %04o",
			errRabbitVRFDKGRuntimeV1,
			info.Mode().Perm(),
		)
	}

	if info.Size() <= 0 ||
		info.Size() > maxRabbitVRFDKGPasswordFileSizeV1 {
		return "", fmt.Errorf(
			"%w: invalid dkg password file size",
			errRabbitVRFDKGRuntimeV1,
		)
	}

	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf(
			"%w: open dkg password file: %v",
			errRabbitVRFDKGRuntimeV1,
			err,
		)
	}
	defer file.Close()

	openedInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf(
			"%w: stat dkg password file: %v",
			errRabbitVRFDKGRuntimeV1,
			err,
		)
	}

	if !openedInfo.Mode().IsRegular() ||
		!os.SameFile(info, openedInfo) {
		return "", fmt.Errorf(
			"%w: dkg password file changed during validation",
			errRabbitVRFDKGRuntimeV1,
		)
	}

	raw, err := io.ReadAll(io.LimitReader(
		file,
		maxRabbitVRFDKGPasswordFileSizeV1+1,
	))
	if err != nil {
		return "", fmt.Errorf(
			"%w: read dkg password file: %v",
			errRabbitVRFDKGRuntimeV1,
			err,
		)
	}
	defer zeroRabbitVRFDKGBytesV1(raw)

	if len(raw) > maxRabbitVRFDKGPasswordFileSizeV1 {
		return "", fmt.Errorf(
			"%w: dkg password file too large",
			errRabbitVRFDKGRuntimeV1,
		)
	}

	password := strings.TrimRight(string(raw), "\r\n")
	if password == "" {
		return "", fmt.Errorf(
			"%w: empty dkg password file",
			errRabbitVRFDKGRuntimeV1,
		)
	}

	return password, nil
}
