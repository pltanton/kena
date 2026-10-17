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
	return strings.ToLower(strings.Join(strings.Fields(NFC(s)), " "))
}
