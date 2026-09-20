//go:build windows

package rabbitvrfstate

import "golang.org/x/sys/windows"

func syncDirectoryV1(
	path string,
) error {
	encoded, err :=
		windows.UTF16PtrFromString(
			path,
		)
	if err != nil {
		return err
	}

	handle, err :=
		windows.CreateFile(
			encoded,
			windows.GENERIC_READ,
			windows.FILE_SHARE_READ|
				windows.FILE_SHARE_WRITE|
				windows.FILE_SHARE_DELETE,
			nil,
			windows.OPEN_EXISTING,
			windows.FILE_FLAG_BACKUP_SEMANTICS,
			0,
		)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)

	return windows.FlushFileBuffers(
		handle,
	)
}
