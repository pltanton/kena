package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pltanton/kena/internal/app"
	"github.com/pltanton/kena/internal/config"
	"github.com/pltanton/kena/internal/db"
	"github.com/pltanton/kena/internal/passphrase"
	"github.com/pltanton/kena/internal/store"
)

var version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "serve":
		err = runServe(os.Args[2:])
	case "migrate":
		err = runMigrate(os.Args[2:])
	case "token":
		err = runToken(os.Args[2:])
	case "user":
		err = runUser(os.Args[2:], os.Stdout)
	case "version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "kena:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: kena <command> [arguments]

commands:
  serve                                     run the web and MCP server
  migrate                                   apply database migrations
  user create NAME [--admin]                create a user and print their passphrase
  user passwd NAME                          print a new passphrase for a user; logs them out everywhere
  token create --user NAME [--admin] [--label L]   issue a personal access token
  version                                   print the build version`)
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	cfg.Version = version

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("migrating database: %w", err)
	}

	st := store.New(pool)
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           app.New(cfg, st),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("kena: listening", "addr", cfg.Listen, "public_url", cfg.PublicURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		slog.Info("kena: shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down: %w", err)
		}
		return <-errCh
	case err := <-errCh:
		return err
	}
}

func runMigrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	databaseURL := os.Getenv("KENA_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("KENA_DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := db.Open(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("migrating database: %w", err)
	}
	slog.Info("kena: migrations applied")
	return nil
}

func runToken(args []string) error {
	if len(args) == 0 || args[0] != "create" {
		return errors.New(`usage: kena token create --user NAME [--admin] [--label L]`)
	}

	fs := flag.NewFlagSet("token create", flag.ContinueOnError)
	user := fs.String("user", "", "username to issue the token for")
	admin := fs.Bool("admin", false, "grant the user admin privileges")
	label := fs.String("label", "", "a label for the token")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *user == "" {
		return errors.New("--user is required")
	}

	ctx := context.Background()
	st, closeDB, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer closeDB()

	u, err := st.EnsureUser(ctx, *user, *admin)
	if err != nil {
		return fmt.Errorf("ensuring user: %w", err)
	}
	token, _, err := st.IssueToken(ctx, u.ID, *label)
	if err != nil {
		return fmt.Errorf("issuing token: %w", err)
	}

	fmt.Println(token)
	return nil
}

func openStore(ctx context.Context) (*store.Store, func(), error) {
	databaseURL := os.Getenv("KENA_DATABASE_URL")
	if databaseURL == "" {
		return nil, nil, errors.New("KENA_DATABASE_URL is required")
	}
	pool, err := db.Open(ctx, databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("opening database: %w", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("migrating database: %w", err)
	}
	return store.New(pool), pool.Close, nil
}

const userUsage = "usage: kena user create NAME [--admin] | kena user passwd NAME"

func runUser(args []string, out io.Writer) error {
	if len(args) == 0 || (args[0] != "create" && args[0] != "passwd") {
		return errors.New(userUsage)
	}
	cmd := args[0]
	var name string
	admin := false
	for _, a := range args[1:] {
		switch {
		case (a == "--admin" || a == "-admin") && cmd == "create":
			admin = true
		case strings.HasPrefix(a, "-") || name != "":
			return errors.New(userUsage)
		default:
			name = a
		}
	}
	if name == "" {
		return errors.New(userUsage)
	}

	ctx := context.Background()
	st, closeDB, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer closeDB()

	var u store.User
	if cmd == "create" {
		u, err = st.CreateUser(ctx, name, admin)
		if errors.Is(err, store.ErrConflict) {
			return fmt.Errorf("user %q already exists; use kena user passwd %s", name, name)
		}
	} else {
		u, err = st.UserByName(ctx, name)
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("user %q not found", name)
		}
	}
	if err != nil {
		return err
	}

	pass, err := passphrase.Generate()
	if err != nil {
		return err
	}
	if err := st.SetPassword(ctx, u.ID, pass, ""); err != nil {
		return fmt.Errorf("setting passphrase: %w", err)
	}
	_, err = fmt.Fprintln(out, pass)
	return err
}
