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
	tmpDir string
	port   int
	cmd    *exec.Cmd
}

func (c *cluster) connString(dbName string) string {
	return fmt.Sprintf("host=%s port=%d user=postgres dbname=%s", c.tmpDir, c.port, dbName)
}

var (
	clusterOnce sync.Once
	sharedClus  *cluster
	clusterErr  error
)

func sharedCluster(t testing.TB) *cluster {
	t.Helper()
	if _, err := exec.LookPath("initdb"); err != nil {
		t.Skip("dbtest: initdb not found on PATH and KENA_TEST_DATABASE_URL not set")
	}
	if _, err := exec.LookPath("postgres"); err != nil {
		t.Skip("dbtest: postgres not found on PATH and KENA_TEST_DATABASE_URL not set")
	}
	clusterOnce.Do(func() {
		sharedClus, clusterErr = startCluster()
	})
	if clusterErr != nil {
		t.Fatalf("dbtest: starting postgres cluster: %v", clusterErr)
	}
	return sharedClus
}

func startCluster() (*cluster, error) {
	tmpDir, err := os.MkdirTemp("", "kpg")
	if err != nil {
		return nil, fmt.Errorf("mkdir temp: %w", err)
	}

	initCmd := exec.Command("initdb", "-A", "trust", "-U", "postgres", "--no-sync", "-D", tmpDir)
	if out, err := initCmd.CombinedOutput(); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("initdb: %w: %s", err, out)
	}

	port := 10000 + rand.IntN(50000)

	logFile, err := os.Create(filepath.Join(tmpDir, "postgres.log"))
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("creating log file: %w", err)
	}

	cmd := exec.Command("postgres",
		"-D", tmpDir,
		"-k", tmpDir,
		"-c", "listen_addresses=",
		"-p", strconv.Itoa(port),
		"-F",
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("starting postgres: %w", err)
	}

	c := &cluster{tmpDir: tmpDir, port: port, cmd: cmd}

	if err := waitReady(c, 10*time.Second); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		logFile.Close()
		os.RemoveAll(tmpDir)
		return nil, err
	}

	return c, nil
}

func waitReady(c *cluster, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var lastErr error
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("postgres did not become ready within %s: %w", timeout, lastErr)
		default:
		}
		conn, err := pgx.Connect(ctx, c.connString("postgres"))
		if err == nil {
			conn.Close(ctx)
			return nil
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
}

// Main wraps testing.M so the shared throwaway cluster started by New is
// stopped and its temp dir removed after the test binary finishes.
func Main(m *testing.M) {
	code := m.Run()
	stopCluster()
	os.Exit(code)
}

func stopCluster() {
	if sharedClus == nil {
		return
	}
	if sharedClus.cmd != nil && sharedClus.cmd.Process != nil {
		_ = sharedClus.cmd.Process.Signal(syscall.SIGTERM)
		_ = sharedClus.cmd.Wait()
	}
	os.RemoveAll(sharedClus.tmpDir)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := crand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// New returns a pool connected to a freshly created, migrated database.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	pool, _ := newDatabase(t)
	if err := db.Migrate(context.Background(), pool); err != nil {
		pool.Close()
		t.Fatalf("dbtest: migrating: %v", err)
	}
	return pool
}

// NewURL returns the connection string of a freshly created, unmigrated
// database, for tests that exercise a command's own migration step.
func NewURL(t testing.TB) string {
	t.Helper()
	pool, url := newDatabase(t)
	pool.Close()
	return url
}

func newDatabase(t testing.TB) (*pgxpool.Pool, string) {
	t.Helper()
	ctx := context.Background()

	adminURL := os.Getenv("KENA_TEST_DATABASE_URL")
	if adminURL == "" {
		c := sharedCluster(t)
		adminURL = c.connString("postgres")
	}

	adminCfg, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		t.Fatalf("dbtest: parsing database url: %v", err)
	}

	adminPool, err := pgxpool.NewWithConfig(ctx, adminCfg)
	if err != nil {
		t.Fatalf("dbtest: connecting to admin database: %v", err)
	}
	defer adminPool.Close()

	dbName := "t_" + randomHex(8)
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatalf("dbtest: creating database %s: %v", dbName, err)
	}

	dbCfg := adminCfg.Copy()
	dbCfg.ConnConfig.Database = dbName

	pool, err := pgxpool.NewWithConfig(ctx, dbCfg)
	if err != nil {
		t.Fatalf("dbtest: connecting to %s: %v", dbName, err)
	}

	t.Cleanup(func() {
		pool.Close()
		dropCtx := context.Background()
		dropPool, err := pgxpool.NewWithConfig(dropCtx, adminCfg)
		if err != nil {
			return
		}
		defer dropPool.Close()
		_, _ = dropPool.Exec(dropCtx, "DROP DATABASE IF EXISTS "+pgx.Identifier{dbName}.Sanitize())
	})

	return pool, withDatabase(adminURL, dbName)
}

func withDatabase(connString, dbName string) string {
	if u, err := url.Parse(connString); err == nil && u.Scheme != "" {
		u.Path = "/" + dbName
		return u.String()
	}
	return connString + " dbname=" + dbName
}
