// Command migrate applies the SQL files in ./migrations in filename order and records each one in
// schema_migrations, so a file never runs twice.
//
//	go run ./cmd/migrate                  dry run: show target DB and pending files
//	go run ./cmd/migrate -yes             apply pending files (each in its own transaction)
//	go run ./cmd/migrate -baseline 21 -yes  record files 000..021 as already applied WITHOUT running them
//
// Target DB comes from DATABASE_URL (.env is loaded if present). The first run on a database that already has
// tables must use -baseline: the early files are not safe to re-run (see CLAUDE.md, 2026-10-25 notes).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

const createTracking = `
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    version    TEXT        PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    baseline   BOOLEAN     NOT NULL DEFAULT false
)`

func main() {
	dir := flag.String("dir", "migrations", "directory with NNN_name.sql files")
	baseline := flag.Int("baseline", -1, "record files with number <= N as applied without running them")
	yes := flag.Bool("yes", false, "actually write (default is a dry run)")
	flag.Parse()

	_ = godotenv.Load()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:root@localhost:5432/postgres?sslmode=disable"
	}
	target := dsn
	if u, err := url.Parse(dsn); err == nil {
		target = u.Host + u.Path // never print credentials
	}
	fmt.Println("target database:", target)

	files, err := filepath.Glob(filepath.Join(*dir, "*.sql"))
	if err != nil {
		log.Fatal(err)
	}
	sort.Strings(files)

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		log.Fatal("connect: ", err)
	}
	defer conn.Close(ctx)

	var hasTracking bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&hasTracking); err != nil {
		log.Fatal(err)
	}
	applied := map[string]bool{}
	if hasTracking {
		rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations`)
		if err != nil {
			log.Fatal(err)
		}
		for rows.Next() {
			var v string
			rows.Scan(&v)
			applied[v] = true
		}
		rows.Close()
	}

	var tables int
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_name <> 'schema_migrations'`).Scan(&tables); err != nil {
		log.Fatal(err)
	}
	if len(applied) == 0 && tables > 0 && *baseline < 0 {
		log.Fatalf("database already has %d tables but no migration history: re-run with -baseline N (last file number already in this DB)", tables)
	}

	type item struct {
		name string
		num  int
	}
	var pending []item
	for _, f := range files {
		name := filepath.Base(f)
		if applied[name] {
			continue
		}
		num, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			log.Fatalf("file %s must start with a number and underscore", name)
		}
		pending = append(pending, item{name, num})
	}

	if len(pending) == 0 {
		fmt.Println("up to date:", len(applied), "migrations recorded")
		return
	}
	for _, p := range pending {
		mode := "APPLY"
		if p.num <= *baseline {
			mode = "BASELINE (record only)"
		}
		fmt.Printf("  pending: %-45s %s\n", p.name, mode)
	}
	if !*yes {
		fmt.Println("dry run — nothing written. Add -yes to proceed.")
		return
	}

	if _, err := conn.Exec(ctx, createTracking); err != nil {
		log.Fatal(err)
	}
	for _, p := range pending {
		isBase := p.num <= *baseline
		tx, err := conn.Begin(ctx)
		if err != nil {
			log.Fatal(err)
		}
		if !isBase {
			body, err := os.ReadFile(filepath.Join(*dir, p.name))
			if err != nil {
				log.Fatal(err)
			}
			if _, err := tx.Exec(ctx, string(body)); err != nil {
				tx.Rollback(ctx)
				log.Fatalf("%s failed (rolled back, later files not run): %v", p.name, err)
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, baseline) VALUES ($1,$2)`, p.name, isBase); err != nil {
			tx.Rollback(ctx)
			log.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			log.Fatal(err)
		}
		fmt.Println("  done:", p.name)
	}
}
