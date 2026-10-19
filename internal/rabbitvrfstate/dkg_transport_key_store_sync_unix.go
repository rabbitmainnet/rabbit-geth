//go:build !windows

package rabbitvrfstate

import "os"

func syncDirectoryV1(
	path string,
) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()

	return directory.Sync()
}
