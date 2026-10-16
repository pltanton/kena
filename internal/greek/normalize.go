package greek

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

type Blank struct {
	Answers []string `json:"answers"`
	Hint    string   `json:"hint,omitempty"`
}

const Marker = "___"

func NFC(s string) string {
	return norm.NFC.String(s)
}

func NormalizeAnswer(s string) string {
	s = NFC(s)
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	return strings.ToLower(s)
}
