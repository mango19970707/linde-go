//go:build !windows

package sessionrecorder

import "os"

func atomicReplaceFile(tempPath, targetPath string) error {
	return os.Rename(tempPath, targetPath)
}
