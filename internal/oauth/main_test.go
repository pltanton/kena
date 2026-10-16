package oauth

import (
	"testing"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/session"
)

func TestMain(m *testing.M) {
	session.FailDelay = 0
	dbtest.Main(m)
}
