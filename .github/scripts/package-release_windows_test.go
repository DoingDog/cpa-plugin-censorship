package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"
)

func TestUnicodeNamesAliasWindowsCaseRules(t *testing.T) {
	t.Run("default directory", func(t *testing.T) {
		dir := t.TempDir()
		for _, tc := range []struct {
			first, second string
			want          bool
		}{
			{"É", "é", true},
			{"K", "K", false},
			{"Σ", "ς", false},
			{"S", "ſ", false},
			{"𐐀", "𐐨", false},
		} {
			t.Run(tc.first+"/"+tc.second, func(t *testing.T) {
				if actual := filesystemNamesAlias(t, dir, tc.first, tc.second); actual != tc.want {
					t.Fatalf("file identity = %v, fixture wants %v", actual, tc.want)
				}
				got, err := unicodeNamesAlias(dir, tc.first, tc.second)
				if err != nil || got != tc.want {
					t.Fatalf("unicodeNamesAlias = %v, %v; want %v", got, err, tc.want)
				}
			})
		}
	})
	t.Run("case-sensitive directory", func(t *testing.T) {
		dir := caseSensitivePackageDirectory(t)
		if filesystemNamesAlias(t, dir, "É", "é") {
			t.Fatal("case-sensitive directory aliases É/é")
		}
		if got, err := unicodeNamesAlias(dir, "É", "é"); err != nil || got {
			t.Fatalf("unicodeNamesAlias = %v, %v; want distinct", got, err)
		}
		testPathsAliasAbsentUnicodeOutputs(t, dir)
		testDirectPackageAbsentUnicodeOutputs(t, dir)
	})
}

func caseSensitivePackageDirectory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(path, 0x80|0x100, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	setInfo := syscall.NewLazyDLL("kernel32.dll").NewProc("SetFileInformationByHandle")
	if err := setInfo.Find(); err != nil {
		if closeErr := syscall.CloseHandle(handle); closeErr != nil {
			t.Fatal(closeErr)
		}
		t.Skipf("SetFileInformationByHandle unavailable: %v", err)
	}
	flags := uint32(1)
	result, _, callErr := setInfo.Call(uintptr(handle), 23, uintptr(unsafe.Pointer(&flags)), unsafe.Sizeof(flags))
	if err := syscall.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	if result == 0 {
		if errors.Is(callErr, syscall.Errno(1)) || errors.Is(callErr, syscall.Errno(5)) || errors.Is(callErr, syscall.Errno(50)) || errors.Is(callErr, syscall.Errno(87)) {
			t.Skipf("FileCaseSensitiveInfo cannot be enabled on this directory: %v", callErr)
		}
		t.Fatalf("SetFileInformationByHandle(FileCaseSensitiveInfo): %v", callErr)
	}
	return dir
}

func TestUnicodeNamesAliasDirectoryError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "removed")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	got, err := unicodeNamesAlias(dir, "É", "é")
	if got || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unicodeNamesAlias removed directory = %v, %v; want not-exist error", got, err)
	}
}

func TestUnicodeNamesAliasInvalidNames(t *testing.T) {
	for _, names := range [][2]string{{"É\x00", "é"}, {"É", "é\x00"}} {
		if got, err := unicodeNamesAlias(t.TempDir(), names[0], names[1]); got || !errors.Is(err, syscall.EINVAL) {
			t.Fatalf("unicodeNamesAlias(%q, %q) = %v, %v; want EINVAL", names[0], names[1], got, err)
		}
	}
}
