package greek

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type ErrorKind string

const (
	KindCorrect    ErrorKind = "correct"
	KindEmpty      ErrorKind = "empty"
	KindAccent     ErrorKind = "accent"
	KindFinalSigma ErrorKind = "final_sigma"
	KindNearMiss   ErrorKind = "near_miss"
	KindOther      ErrorKind = "other"
)

const nearMissDistance = 2

var kindRank = map[ErrorKind]int{KindCorrect: 0, KindAccent: 1, KindFinalSigma: 2, KindNearMiss: 3, KindOther: 4}

func Classify(given string, accepted []string) ErrorKind {
	g := NormalizeAnswer(given)
	if g == "" {
		return KindEmpty
	}
	best := KindOther
	for _, a := range accepted {
		if k := classifyOne(g, NormalizeAnswer(a)); kindRank[k] < kindRank[best] {
			best = k
		}
	}
	return best
}

func classifyOne(g, a string) ErrorKind {
	switch {
	case g == a:
		return KindCorrect
	case stripAccents(g) == stripAccents(a):
		return KindAccent
	case foldSigma(g) == foldSigma(a):
		return KindFinalSigma
	case levenshtein(stripAccents(g), stripAccents(a)) <= nearMissDistance:
		return KindNearMiss
	}
	return KindOther
}

func stripAccents(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func foldSigma(s string) string {
	return strings.ReplaceAll(s, "ς", "σ")
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
