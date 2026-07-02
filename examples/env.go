package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var loadExampleEnvOnce sync.Once

func GetEnv(key string) string {
	loadExampleEnv()
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	panic(fmt.Sprintf("missing required env %s; set it in examples/.env or your shell environment", key))
}

func GetEnvWith(key, fallback string) string {
	loadExampleEnv()
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func loadExampleEnv() {
	loadExampleEnvOnce.Do(func() {
		loadEnvFile(".env")
		loadEnvFile(filepath.Join("examples", ".env"))
		loadEnvFile(filepath.Join("..", ".env"))
	})
}

func loadEnvFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" || value == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, value)
	}
}
