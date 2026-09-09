package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"vowfilm/server/internal/config"
	"vowfilm/server/internal/storage"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: export-project PROJECT_ID")
	}
	envFile := os.Getenv("VOWFILM_ENV_FILE")
	if envFile == "" {
		envFile = "../.env"
	}
	if err := config.LoadEnv(envFile); err != nil {
		log.Fatal(err)
	}
	driver := os.Getenv("DATABASE_DRIVER")
	if driver == "" {
		driver = "sqlite"
	}
	dsn := os.Getenv("DATABASE_URL")
	if driver == "sqlite" && dsn == "" {
		data := os.Getenv("VOWFILM_DATA_DIR")
		if data == "" {
			data = "../data"
		}
		dsn = filepath.Join(data, "platform.sqlite")
	}
	repo, err := storage.Open(driver, dsn)
	if err != nil {
		log.Fatal("Database unavailable")
	}
	defer repo.Close()
	projects, err := repo.LoadProjects()
	if err != nil {
		log.Fatal(err)
	}
	p := projects[os.Args[1]]
	if p == nil {
		log.Fatal("Project not found")
	}
	if err = json.NewEncoder(os.Stdout).Encode(p); err != nil {
		log.Fatal(err)
	}
}
