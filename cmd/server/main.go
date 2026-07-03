package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"v.wingsnet.org/internal/config"
	"v.wingsnet.org/internal/httpapi"
	"v.wingsnet.org/internal/pki"
	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/migrate"
)

func main() {
	cfg := config.Load()
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "serve" {
		runServe(cfg)
		return
	}
	switch args[0] {
	case "db":
		runDB(cfg, args[1:])
	case "ca":
		runCA(cfg, args[1:])
	default:
		log.Fatalf("unknown command %q; use one of: serve, db, ca", args[0])
	}
}

func runServe(cfg config.Config) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("v.wingsnet.org listening on %s", cfg.ListenAddr)
	if err := httpapi.Run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}

func runDB(cfg config.Config, args []string) {
	const usage = "usage: wingsv-panel db migrate <src> <dst> | db migrate -f <file.db> <dst>"
	if len(args) == 0 || args[0] != "migrate" {
		log.Fatal(usage)
	}
	rest := args[1:]

	var srcOpts storage.Options
	var dstName string
	switch {
	case len(rest) == 3 && rest[0] == "-f":
		srcOpts = storage.Options{Driver: storage.DriverSQLite, DSN: rest[1]}
		dstName = rest[2]
	case len(rest) == 2:
		opts, err := driverOptions(cfg, rest[0])
		if err != nil {
			log.Fatal(err)
		}
		srcOpts = opts
		dstName = rest[1]
	default:
		log.Fatal(usage)
	}

	dstOpts, err := driverOptions(cfg, dstName)
	if err != nil {
		log.Fatal(err)
	}
	results, err := migrate.Run(srcOpts, dstOpts)
	if err != nil {
		log.Fatalf("migrate: %v", err)
	}
	var total int64
	for _, r := range results {
		fmt.Printf("  %-24s %d\n", r.Table, r.Rows)
		total += r.Rows
	}
	fmt.Printf("migrated %d rows across %d tables\n", total, len(results))
}

func driverOptions(cfg config.Config, name string) (storage.Options, error) {
	driver, err := storage.NormalizeDriver(name)
	if err != nil {
		return storage.Options{}, err
	}
	if driver == storage.DriverSQLite {
		return storage.Options{Driver: driver, DSN: cfg.DBPath}, nil
	}
	if cfg.DBDSN == "" {
		return storage.Options{}, fmt.Errorf("driver %s needs DB_DSN set in the environment", driver)
	}
	return storage.Options{Driver: driver, DSN: cfg.DBDSN}, nil
}

func runCA(cfg config.Config, args []string) {
	if len(args) == 0 {
		log.Fatal("usage: wingsv-panel ca <init|show-pin>")
	}
	switch args[0] {
	case "init":
		ca, created, err := pki.LoadOrCreateCA(cfg.CADir)
		if err != nil {
			log.Fatal(err)
		}
		if created {
			fmt.Println("generated new panel CA in", cfg.CADir)
		} else {
			fmt.Println("panel CA already present in", cfg.CADir)
		}
		fmt.Println("pin:", ca.PinBase64())
	case "show-pin":
		ca, err := pki.LoadCADir(cfg.CADir)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(ca.PinBase64())
	default:
		log.Fatalf("unknown ca command %q; use init or show-pin", args[0])
	}
}
