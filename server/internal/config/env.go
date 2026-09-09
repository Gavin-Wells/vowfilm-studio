package config

import (
	"bufio"
	"os"
	"strings"
)

// LoadEnv fills unset process variables; explicit environment values take precedence.
func LoadEnv(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && os.Getenv(k) == "" {
			if err = os.Setenv(k, strings.Trim(v, "\"'")); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}
