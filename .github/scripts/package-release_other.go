//go:build !windows

package main

func unicodeNamesAlias(ancestorPath, firstName, secondName string) (bool, error) {
	return false, nil
}
