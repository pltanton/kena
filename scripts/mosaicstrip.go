//go:build ignore

package main

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
)

type palette struct {
	stone []string
	slate []string
	gold  string
}

func strip(p palette, seed int64) string {
	const width, height, gapAt = 360.0, 8.0, 23
	r := rand.New(rand.NewSource(seed))
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%g" height="%g" viewBox="0 0 %g %g">`, width, height, width, height)
	x, i := 0.6, 0
	for x < width-4 {
		w := 5.4 + r.Float64()*2.2
		if x+w > width-0.6 {
			w = width - 0.6 - x
		}
		h := 5.6 + r.Float64()*1.2
		y := (height-h)/2 + (r.Float64()-0.5)*0.8
		rot := (r.Float64() - 0.5) * 7
		fill := p.stone[r.Intn(len(p.stone))]
		if r.Float64() < 0.12 {
			fill = p.slate[r.Intn(len(p.slate))]
		}
		if i == gapAt {
			fmt.Fprintf(&b, `<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" fill="none" stroke="%s" stroke-width="0.9" stroke-dasharray="1.3 0.9"/>`, x+0.5, (height-h)/2+0.5, w-1, h-1, p.gold)
		} else {
			fmt.Fprintf(&b, `<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" rx="0.4" fill="%s" transform="rotate(%.1f %.2f %.2f)"/>`, x, y, w, h, fill, rot, x+w/2, y+h/2)
		}
		x += w + 0.9 + r.Float64()*0.5
		i++
	}
	b.WriteString("</svg>\n")
	return b.String()
}

func main() {
	light := palette{
		stone: []string{"#e6dcc6", "#ddd0b4", "#e9e0cc", "#d6c8aa", "#e1d5bc"},
		slate: []string{"#2f4a6d", "#3b5a82"},
		gold:  "#c9962e",
	}
	dark := palette{
		stone: []string{"#2b3038", "#30353e", "#272b33", "#343a44", "#2e333b"},
		slate: []string{"#3b5578", "#46628a"},
		gold:  "#d6a943",
	}
	for name, p := range map[string]palette{"strip.svg": light, "strip-dark.svg": dark} {
		if err := os.WriteFile("internal/web/static/brand/"+name, []byte(strip(p, 7)), 0o644); err != nil {
			panic(err)
		}
	}
}
