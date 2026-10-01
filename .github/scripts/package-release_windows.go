package main

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	packagerKernel32           = syscall.NewLazyDLL("kernel32.dll")
	getPackagerFileInformation = packagerKernel32.NewProc("GetFileInformationByHandleEx")
	comparePackagerNames       = packagerKernel32.NewProc("CompareStringOrdinal")
)

func unicodeNamesAlias(ancestorPath, firstName, secondName string) (same bool, err error) {
	path, err := syscall.UTF16PtrFromString(ancestorPath)
	if err != nil {
		return false, fmt.Errorf("encode directory %q: %w", ancestorPath, err)
	}
	handle, err := syscall.CreateFile(path, 0x80, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return false, fmt.Errorf("open directory %q: %w", ancestorPath, err)
	}
	defer func() {
		if closeErr := syscall.CloseHandle(handle); closeErr != nil {
			same = false
			err = errors.Join(err, fmt.Errorf("close directory %q: %w", ancestorPath, closeErr))
		}
	}()
	if err := getPackagerFileInformation.Find(); err != nil {
		return false, fmt.Errorf("find GetFileInformationByHandleEx: %w", err)
	}
	// FileCaseSensitiveInfo = 23，FILE_CASE_SENSITIVE_INFO 只有一个 DWORD Flags。
	var flags uint32
	result, _, callErr := getPackagerFileInformation.Call(uintptr(handle), 23, uintptr(unsafe.Pointer(&flags)), unsafe.Sizeof(flags))
	if result == 0 {
		return false, fmt.Errorf("query FileCaseSensitiveInfo for %q: %w", ancestorPath, callErr)
	}
	if flags&1 != 0 {
		return false, nil
	}
	first, err := syscall.UTF16FromString(firstName)
	if err != nil {
		return false, fmt.Errorf("encode filename %q: %w", firstName, err)
	}
	second, err := syscall.UTF16FromString(secondName)
	if err != nil {
		return false, fmt.Errorf("encode filename %q: %w", secondName, err)
	}
	if err := comparePackagerNames.Find(); err != nil {
		return false, fmt.Errorf("find CompareStringOrdinal: %w", err)
	}
	result, _, callErr = comparePackagerNames.Call(uintptr(unsafe.Pointer(&first[0])), uintptr(len(first)-1), uintptr(unsafe.Pointer(&second[0])), uintptr(len(second)-1), 1)
	switch result {
	case 2: // CSTR_EQUAL
		return true, nil
	case 1, 3: // CSTR_LESS_THAN、CSTR_GREATER_THAN
		return false, nil
	case 0:
		return false, fmt.Errorf("CompareStringOrdinal(%q, %q): %w", firstName, secondName, callErr)
	default:
		return false, fmt.Errorf("CompareStringOrdinal returned unexpected result %d", result)
	}
}
