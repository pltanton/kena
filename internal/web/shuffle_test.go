package web

import (
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/pltanton/kena/internal/store"
)

type shuffleVector struct {
	Seed string  `json:"seed"`
	N    int     `json:"n"`
	Perm []int64 `json:"perm"`
}

const shuffleFile = "testdata/shuffle.json"

// offline.js must order a shuffled session exactly like ordered (internal/web/jstest).
func TestShuffleVectors(t *testing.T) {
	var got []shuffleVector
	for _, seed := range []uint64{0, 1, 42, 3141592653, 4294967295, 18446744073709551615} {
		for _, n := range []int{0, 1, 2, 3, 5, 10, 37} {
			tasks := make([]store.Task, n)
			for i := range tasks {
				tasks[i].ID = int64(i + 1)
			}
			v := shuffleVector{Seed: strconv.FormatUint(seed, 10), N: n, Perm: []int64{}}
			for _, task := range ordered(tasks, params{Shuffle: true, Seed: seed}) {
				v.Perm = append(v.Perm, task.ID)
			}
			got = append(got, v)
		}
	}
	if os.Getenv("KENA_UPDATE_GOLDEN") != "" {
		b, err := json.MarshalIndent(got, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(shuffleFile, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(shuffleFile)
	if err != nil {
		t.Fatal(err)
	}
	var want []shuffleVector
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s is stale; regenerate with KENA_UPDATE_GOLDEN=1", shuffleFile)
	}
}
