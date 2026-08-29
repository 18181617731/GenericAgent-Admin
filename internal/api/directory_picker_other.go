//go:build !windows && !darwin

package api

import "fmt"

func chooseDirectory(string) (string, error) {
	return "", fmt.Errorf("directory picker is not supported on this platform; please paste the path manually")
}
