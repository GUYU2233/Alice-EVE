// Command sde-import-server transactionally imports CCP's JSONL SDE zip.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"relay-server/internal/sdeimport"
)

func main() {
	zipPath := flag.String("zip", "", "path to eve-sde-*-jsonl.zip")
	version := flag.String("version", "", "SDE build/version identifier")
	dsn := flag.String("database-url", os.Getenv("DATABASE_URL"), "PostgreSQL DSN (defaults to DATABASE_URL)")
	flag.Parse()
	if *zipPath == "" || *version == "" || *dsn == "" {
		flag.Usage()
		os.Exit(2)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	counts, err := sdeimport.Import(ctx, pool, *zipPath, *version)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("activated SDE build %s: types=%d market_groups=%d regions=%d constellations=%d systems=%d stargates=%d\n", *version, counts.Types, counts.MarketGroups, counts.Regions, counts.Constellations, counts.Systems, counts.Stargates)
}
