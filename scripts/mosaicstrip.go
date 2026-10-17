//go:build ignore

package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
)

type palette struct {
	stone []string
	slate []string
	gold  string
	inlay string
}

func shade(hex string, f float64) string {
	v, _ := strconv.ParseUint(hex[1:], 16, 32)
	out := "#"
	for _, c := range []uint64{v >> 16, v >> 8 & 0xff, v & 0xff} {
		x := float64(c) * f
		if x > 255 {
			x = 255
		}
		out += fmt.Sprintf("%02x", int(x+0.5))
	}
	return out
}

func tileWidth(r *rand.Rand) float64 {
	switch p := r.Float64(); {
	case p < 0.12:
		return 2.8 + r.Float64()*1.4
	case p < 0.85:
		return 4.4 + r.Float64()*4
	default:
		return 8.5 + r.Float64()*4.5
	}
}

func strip(p palette, seed int64) string {
	const width, height = 1680.0, 8.0
	r := rand.New(rand.NewSource(seed))
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%g" height="%g" viewBox="0 0 %g %g">`, width, height, width, height)
	x := 0.4 + r.Float64()*0.4
	nextAccent := 6 + r.Intn(30)
	missingAt := width * (0.3 + r.Float64()*0.4)
	missing := false
	for i := 0; x < width-3; i++ {
		w := tileWidth(r)
		if x+w > width-0.5 {
			w = width - 0.5 - x
		}
		h := 5.0 + r.Float64()*2.0
		y := (height-h)/2 + (r.Float64()-0.5)*1.2
		rot := (r.Float64() - 0.5) * 40 / (w + 5)
		fill := shade(p.stone[r.Intn(len(p.stone))], 0.95+r.Float64()*0.07)
		if i == nextAccent {
			fill = p.slate[r.Intn(len(p.slate))]
			if r.Float64() < 0.2 {
				fill = p.inlay
			}
			nextAccent = i + 5 + r.Intn(45)
		}
		if !missing && x > missingAt {
			missing = true
			fmt.Fprintf(&b, `<rect x="%.1f" y="1.4" width="5.6" height="5.2" fill="none" stroke="%s" stroke-width="0.7" stroke-dasharray="1.1 0.9" opacity="0.85"/>`, x+0.4, p.gold)
			x += 7 + r.Float64()*1.1
			continue
		}
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.2f" width="%.1f" height="%.1f" rx="0.4" fill="%s" transform="rotate(%.1f %.1f %.1f)"/>`, x, y, w, h, fill, rot, x+w/2, y+h/2)
		x += w + 0.55 + r.Float64()*r.Float64()*1.6
	}
	b.WriteString("</svg>\n")
	return b.String()
}

func main() {
	light := palette{
		stone: []string{"#e6dcc6", "#ddd0b4", "#e9e0cc", "#d6c8aa", "#e1d5bc", "#d9cbb0", "#cfc0a2", "#d2c4a6"},
		slate: []string{"#2f4a6d", "#3b5a82", "#4a6990"},
		gold:  "#c9962e",
		inlay: "#cfa64e",
	}
	dark := palette{
		stone: []string{"#2b3038", "#30353e", "#272b33", "#343a44", "#2e333b", "#383e48", "#25292f"},
		slate: []string{"#3b5578", "#46628a", "#506d94"},
		gold:  "#d6a943",
		inlay: "#a8853a",
	}
	for name, p := range map[string]palette{"strip.svg": light, "strip-dark.svg": dark} {
		if err := os.WriteFile("internal/web/static/brand/"+name, []byte(strip(p, 11)), 0o644); err != nil {
			panic(err)
		}
	}
}
