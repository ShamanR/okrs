// Independent reference runner; executed in a temporary module by generate.sh.
package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"github.com/golang-migrate/migrate/v4/database"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"log"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	if os.Args[1] == "key" {
		key, err := database.GenerateAdvisoryLockId(os.Args[2], os.Args[3], "schema_migrations")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(key)
		return
	}
	if os.Args[1] == "lock" {
		db, err := sql.Open("postgres", os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
		d, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
		if err != nil {
			log.Fatal(err)
		}
		if err = d.Lock(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("locked")
		bufio.NewReader(os.Stdin).ReadString('\n')
		if err = d.Unlock(); err != nil {
			log.Fatal(err)
		}
		d.Close()
		return
	}

	version, err := strconv.ParseUint(os.Args[3], 10, 32)
	if err != nil {
		log.Fatal(err)
	}
	m, err := migrate.New("file://"+os.Args[1], os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	defer m.Close()
	if err := m.Migrate(uint(version)); err != nil && err != migrate.ErrNoChange {
		log.Fatal(err)
	}
}
