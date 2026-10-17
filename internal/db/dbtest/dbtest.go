package dbtest

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pltanton/kena/internal/db"
)

type cluster struct {
	dir string
	cmd *exec.Cmd
	url string
}

var (
	clusterOnce sync.Once
	sharedClus  *cluster
	clusterErr  error

	templateOnce sync.Once
	templateName string
	templateErr  error
)

func adminURL(t testing.TB) string {
	t.Helper()
	if u := os.Getenv("KENA_TEST_DATABASE_URL"); u != "" {
		return u
	}
	for _, bin := range []string{"initdb", "postgres"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("dbtest: %s not found on PATH and KENA_TEST_DATABASE_URL not set", bin)
		}
	}
	clusterOnce.Do(func() { sharedClus, clusterErr = startCluster() })
	if clusterErr != nil {
		t.Fatalf("dbtest: starting postgres cluster: %v", clusterErr)
	}
	return sharedClus.url
}

func startCluster() (*cluster, error) {
	dir, err := os.MkdirTemp("", "kpg")
	if err != nil {
		return nil, fmt.Errorf("mkdir temp: %w", err)
	}
	c := &cluster{dir: dir}
	if out, err := exec.Command("initdb", "-A", "trust", "-U", "postgres", "--no-sync", "-D", dir).CombinedOutput(); err != nil {
		c.stop()
		return nil, fmt.Errorf("initdb: %w: %s", err, out)
	}
	logFile, err := os.Create(filepath.Join(dir, "postgres.log"))
	if err != nil {
		c.stop()
		return nil, fmt.Errorf("creating log file: %w", err)
	}
	defer logFile.Close()
	port := 10000 + rand.IntN(50000)
	c.url = fmt.Sprintf("host=%s port=%d user=postgres dbname=postgres", dir, port)
	c.cmd = exec.Command("postgres", "-D", dir, "-k", dir, "-c", "listen_addresses=", "-p", strconv.Itoa(port), "-F")
	c.cmd.Stdout, c.cmd.Stderr = logFile, logFile
	if err := c.cmd.Start(); err != nil {
		c.cmd = nil
		c.stop()
		return nil, fmt.Errorf("starting postgres: %w", err)
	}
	if err := waitReady(c.url, 10*time.Second); err != nil {
		c.stop()
		return nil, err
	}
	return c, nil
}

func waitReady(connString string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		conn, err := pgx.Connect(ctx, connString)
		if err == nil {
			return conn.Close(ctx)
		}
		if ctx.Err() != nil {
			return fmt.Errorf("postgres did not become ready within %s: %w", timeout, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (c *cluster) stop() {
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Signal(syscall.SIGTERM)
		_ = c.cmd.Wait()
	}
	os.RemoveAll(c.dir)
}

// Main stops the shared cluster and drops the migrated template database
// once the test binary finishes.
func Main(m *testing.M) {
	code := m.Run()
	if sharedClus != nil {
		sharedClus.stop()
	} else if templateName != "" {
		_ = adminExec(os.Getenv("KENA_TEST_DATABASE_URL"), "DROP DATABASE IF EXISTS "+pgx.Identifier{templateName}.Sanitize())
	}
	os.Exit(code)
}

func adminExec(admin, sql string) error {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, sql)
	return err
}

func randomName(prefix string) string {
	b := make([]byte, 8)
	if _, err := crand.Read(b); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b)
}

// New returns a pool connected to a freshly created, migrated database.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	admin := adminURL(t)
	templateOnce.Do(func() { templateName, templateErr = migratedTemplate(admin) })
	if templateErr != nil {
		t.Fatalf("dbtest: preparing migrated template: %v", templateErr)
	}
	connString := createDatabase(t, admin, templateName)
	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		t.Fatalf("dbtest: connecting: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// NewURL returns the connection string of a freshly created, unmigrated
// database, for tests that exercise a command's own migration step.
func NewURL(t testing.TB) string {
	t.Helper()
	return createDatabase(t, adminURL(t), "template1")
}

func migratedTemplate(admin string) (string, error) {
	name := randomName("tmpl_")
	if err := adminExec(admin, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return "", fmt.Errorf("creating template: %w", err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, withDatabase(admin, name))
	if err != nil {
		return name, err
	}
	defer pool.Close()
	return name, db.Migrate(ctx, pool)
}

func createDatabase(t testing.TB, admin, template string) string {
	t.Helper()
	name := randomName("t_")
	quoted := pgx.Identifier{name}.Sanitize()
	if err := adminExec(admin, "CREATE DATABASE "+quoted+" TEMPLATE "+pgx.Identifier{template}.Sanitize()); err != nil {
		t.Fatalf("dbtest: creating database: %v", err)
	}
	t.Cleanup(func() { _ = adminExec(admin, "DROP DATABASE IF EXISTS "+quoted) })
	return withDatabase(admin, name)
}

func withDatabase(connString, dbName string) string {
	if u, err := url.Parse(connString); err == nil && u.Scheme != "" {
		u.Path = "/" + dbName
		return u.String()
	}
	return connString + " dbname=" + dbName
}
