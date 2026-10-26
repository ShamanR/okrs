// Independent migration job used by enterprise-smoke.py. Never runs the server.
package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"strconv"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"okrs/internal/platform/migrations"
)

func main() {
	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if len(os.Args) < 2 {
		log.Fatal("expected app or goose")
	}
	switch os.Args[1] {
	case "app":
		if len(os.Args) == 3 {
			version, err := strconv.ParseInt(os.Args[2], 10, 64)
			if err != nil {
				log.Fatal(err)
			}
			err = migrations.To(context.Background(), db, os.DirFS("/app/migrations"), version)
			if err != nil {
				log.Fatal(err)
			}
		} else if err := migrations.Up(context.Background(), db, os.DirFS("/app/migrations")); err != nil {
			log.Fatal(err)
		}
	case "goose":
		// Same public entry point as the ordinary goose CLI, without the app bootstrap.
		if err := goose.SetDialect("postgres"); err != nil {
			log.Fatal(err)
		}
		if err := goose.UpContext(context.Background(), db, "/app/migrations"); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatal("expected app or goose")
	}
}
