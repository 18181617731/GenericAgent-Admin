//go:build darwin && !cgo

package api

import "fmt"

func chooseDirectory(string) (string, error) {
	return "", fmt.Errorf("directory picker requires a macOS build with CGO enabled; please paste the path manually")
}
