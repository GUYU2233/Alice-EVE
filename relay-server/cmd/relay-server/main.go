package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"relay-server/internal/api"
	"relay-server/internal/store"
)

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:18443"
	}
	ctx := context.Background()
	var s *api.Server
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		pg, err := store.NewPostgres(ctx, dsn)
		if err != nil {
			log.Fatal(err)
		}
		defer pg.Close()
		s = api.NewServer(pg)
		log.Println("using PostgreSQL store")
	} else {
		log.Println("DATABASE_URL unset; using in-memory store")
		s = api.NewServer()
	}
	log.Printf("relay listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, s.Handler()))
}
