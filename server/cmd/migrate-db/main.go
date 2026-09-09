package main

import (
	"flag"
	"log"
	"os"
	"vowfilm/server/internal/storage"
)

func main() {
	sourceDriver := flag.String("source-driver", "sqlite", "sqlite or postgres")
	sourceURL := flag.String("source-url", "", "source SQLite path; PostgreSQL URL may be supplied in MIGRATE_SOURCE_URL")
	targetDriver := flag.String("target-driver", "postgres", "sqlite or postgres")
	flag.Parse()
	sourceDSN := *sourceURL
	if sourceDSN == "" {
		sourceDSN = os.Getenv("MIGRATE_SOURCE_URL")
	}
	targetDSN := os.Getenv("MIGRATE_TARGET_URL")
	if sourceDSN == "" || targetDSN == "" {
		log.Fatal("Provide source location and MIGRATE_TARGET_URL; stop the application before migration")
	}
	if *sourceDriver == *targetDriver && sourceDSN == targetDSN {
		log.Fatal("Source and target must differ")
	}
	source, err := storage.Open(*sourceDriver, sourceDSN)
	if err != nil {
		log.Fatal("Source connection failed; check configuration")
	}
	defer source.Close()
	target, err := storage.Open(*targetDriver, targetDSN)
	if err != nil {
		log.Fatal("Target connection failed; check configuration")
	}
	defer target.Close()
	if err = source.TransferTo(target); err != nil {
		log.Fatal(err)
	}
	log.Print("Migration complete. Account, ledger, pricing, project and session records copied. Media remains in VOWFILM_DATA_DIR.")
}
