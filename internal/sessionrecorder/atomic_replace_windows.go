//go:build windows

package sessionrecorder

import (
	"errors"
	"golang.org/x/sys/windows"
	"unsafe"
)

const (
	replaceFileWriteThrough = 0x00000001
	moveFileReplaceExisting = 0x00000001
	moveFileWriteThrough    = 0x00000008
)

var (
	kernel32ProcReplaceFile = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReplaceFileW")
	kernel32ProcMoveFileEx  = windows.NewLazySystemDLL("kernel32.dll").NewProc("MoveFileExW")
)

func atomicReplaceFile(tempPath, targetPath string) error {
	tempPtr, err := windows.UTF16PtrFromString(tempPath)
	if err != nil {
		return err
	}
	targetPtr, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return err
	}

	if err := replaceFile(targetPtr, tempPtr); err == nil {
		return nil
	} else if err != windows.ERROR_FILE_NOT_FOUND && err != windows.ERROR_PATH_NOT_FOUND {
		return err
	}

	return moveFileEx(tempPtr, targetPtr)
}

func replaceFile(targetPtr, tempPtr *uint16) error {
	r1, _, err := kernel32ProcReplaceFile.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		uintptr(unsafe.Pointer(tempPtr)),
		0,
		uintptr(replaceFileWriteThrough),
		0,
		0,
	)
	if r1 != 0 {
		return nil
	}
	if err != nil && err != windows.Errno(0) {
		return err
	}
	return errors.New("ReplaceFileW failed")
}

func moveFileEx(tempPtr, targetPtr *uint16) error {
	r1, _, err := kernel32ProcMoveFileEx.Call(
		uintptr(unsafe.Pointer(tempPtr)),
		uintptr(unsafe.Pointer(targetPtr)),
		uintptr(moveFileReplaceExisting|moveFileWriteThrough),
	)
	if r1 != 0 {
		return nil
	}
	if err != nil && err != windows.Errno(0) {
		return err
	}
	return errors.New("MoveFileExW failed")
}
