package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"v.wingsnet.org/internal/config"
	"v.wingsnet.org/internal/httpapi"
	"v.wingsnet.org/internal/pki"
	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
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
	case "connect":
		runConnect(cfg, args[1:])
	case "node":
		runNode(cfg, args[1:])
	default:
		log.Fatalf("unknown command %q; use one of: serve, db, ca, connect, node", args[0])
	}
}

// runNode registers a server node directly in the DB (no HTTP/login), for the
// installer. Prints {"node_id","grpc_token",...} as one JSON line. Idempotent by
// (kind, name): re-running returns the existing node instead of duplicating it.
func runNode(cfg config.Config, args []string) {
	usage := "usage: wingsv-panel node add --kind vk_turn_proxy|xui --name <n> " +
		"--grpc-endpoint <host:port> [--grpc-token <t>] [--wg-backend own|xui] " +
		"[--xui-node-id <id>] [--xui-inbound-tag <tag>]"
	if len(args) == 0 || args[0] != "add" {
		log.Fatal(usage)
	}
	fs := flag.NewFlagSet("node add", flag.ExitOnError)
	kind := fs.String("kind", "", "vk_turn_proxy|xui (aliases: vktp)")
	name := fs.String("name", "", "display name")
	endpoint := fs.String("grpc-endpoint", "", "node gRPC endpoint host:port")
	token := fs.String("grpc-token", "", "panel->node bearer/AEAD token (generated if empty)")
	wgBackend := fs.String("wg-backend", "", "own|xui")
	xuiNodeID := fs.String("xui-node-id", "", "3x-ui node id (wg-backend=xui)")
	xuiInboundTag := fs.String("xui-inbound-tag", "", "3x-ui inbound tag (wg-backend=xui)")
	if err := fs.Parse(args[1:]); err != nil {
		log.Fatal(err)
	}
	k := normalizeNodeKind(*kind)
	if k == "" {
		log.Fatal("--kind must be vk_turn_proxy (alias vktp) or xui")
	}
	if strings.TrimSpace(*endpoint) == "" {
		log.Fatal("--grpc-endpoint is required")
	}
	opts, err := driverOptions(cfg, cfg.DBKind)
	if err != nil {
		log.Fatal(err)
	}
	store, err := storage.Open(opts)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if nm := strings.TrimSpace(*name); nm != "" {
		if existing, found := findNodeByName(store, k, nm); found {
			printNode(existing)
			return
		}
	}
	tok := strings.TrimSpace(*token)
	if tok == "" {
		tok = randomHex(32)
	}
	node, err := store.CreateServerNode(dbmodel.ServerNode{
		ID:            randomHex(16),
		Kind:          k,
		Name:          strings.TrimSpace(*name),
		GRPCEndpoint:  strings.TrimSpace(*endpoint),
		GRPCToken:     tok,
		OwnerAdminID:  0,
		WGBackend:     strings.TrimSpace(*wgBackend),
		XuiNodeID:     strings.TrimSpace(*xuiNodeID),
		XuiInboundTag: strings.TrimSpace(*xuiInboundTag),
	})
	if err != nil {
		log.Fatalf("create node: %v", err)
	}
	printNode(node)
}

func normalizeNodeKind(k string) string {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "vktp", "vk_turn_proxy", "vk-turn-proxy":
		return storage.ServerNodeVKTurnProxy
	case "xui", "3x-ui":
		return storage.ServerNodeXUI
	}
	return ""
}

func findNodeByName(store *storage.Store, kind, name string) (dbmodel.ServerNode, bool) {
	nodes, err := store.ListServerNodes(kind)
	if err != nil {
		return dbmodel.ServerNode{}, false
	}
	for _, n := range nodes {
		if n.Name == name {
			return n, true
		}
	}
	return dbmodel.ServerNode{}, false
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("random: %v", err)
	}
	return hex.EncodeToString(b)
}

func printNode(n dbmodel.ServerNode) {
	out, err := json.Marshal(map[string]string{
		"node_id":    n.ID,
		"grpc_token": n.GRPCToken,
		"kind":       n.Kind,
		"name":       n.Name,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(out))
}

// runConnect prints the ready-to-paste connector command for a registered node,
// so an operator can wire that node to the panel from the panel host's CLI.
func runConnect(cfg config.Config, args []string) {
	if len(args) != 1 {
		log.Fatal("usage: wingsv-panel connect <node-id>")
	}
	opts, err := driverOptions(cfg, cfg.DBKind)
	if err != nil {
		log.Fatal(err)
	}
	store, err := storage.Open(opts)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	node, err := store.GetServerNode(args[0])
	if err != nil {
		log.Fatalf("node %q: %v", args[0], err)
	}
	host := strings.TrimPrefix(strings.TrimPrefix(cfg.PublicBaseURL, "https://"), "http://")
	kind := "vktp"
	if node.Kind == "xui" {
		kind = "xui"
	}
	token := node.GRPCToken
	if token == "" {
		token = "<token>"
	}
	fmt.Printf("curl -fsSL %s/connect.sh | sh -s -- grpc connect %s:443 %s %s %s\n",
		strings.TrimRight(cfg.PublicBaseURL, "/"), host, token, node.ID, kind)
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
