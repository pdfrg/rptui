package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/adrg/xdg"
	"github.com/pelletier/go-toml/v2"
)

type ThemeColors struct {
	Accent     string `toml:"accent"`
	Cursor     string `toml:"cursor"`
	Foreground string `toml:"foreground"`
	Background string `toml:"background"`
	Muted      string `toml:"muted"`
	Color0     string `toml:"color0"`
	Color1     string `toml:"color1"`
	Color2     string `toml:"color2"`
	Color3     string `toml:"color3"`
	Color4     string `toml:"color4"`
	Color5     string `toml:"color5"`
	Color6     string `toml:"color6"`
	Color7     string `toml:"color7"`
	Color8     string `toml:"color8"`
	Color9     string `toml:"color9"`
	Color10    string `toml:"color10"`
	Color11    string `toml:"color11"`
	Color12    string `toml:"color12"`
	Color13    string `toml:"color13"`
	Color14    string `toml:"color14"`
	Color15    string `toml:"color15"`
}

type ThemeInfo struct {
	Name        string
	Background  string
	Foreground  string
	Accent      string
	Muted       string
	Cursor      string
	AccentDist  float64
	CursorDist  float64
	AccentRatio float64
	CursorRatio float64
	MutedDist   float64 // distance from accent to muted
	MutedSat    float64 // saturation of muted (color7)
	FixAccent   string
	FixCursor   string
	FixMuted    string
	// Theme's own ANSI palette (color1-6) for fallbacks
	Colors map[string]string
}

func parseHex(s string) (r, g, b int, ok bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	_, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b)
	return r, g, b, err == nil
}

func weightedDist(h1, h2 string) float64 {
	r1, g1, b1, ok1 := parseHex(h1)
	r2, g2, b2, ok2 := parseHex(h2)
	if !ok1 || !ok2 {
		return -1
	}
	dr, dg, db := float64(r1-r2), float64(g1-g2), float64(b1-b2)
	return math.Sqrt(0.299*dr*dr + 0.587*dg*dg + 0.114*db*db)
}

func contrastRatio(h1, h2 string) float64 {
	rl := func(hex string) float64 {
		r, g, b, ok := parseHex(hex)
		if !ok {
			return 0
		}
		lin := func(c int) float64 {
			v := float64(c) / 255.0
			if v <= 0.03928 {
				return v / 12.92
			}
			return math.Pow((v+0.055)/1.055, 2.4)
		}
		return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
	}
	l1, l2 := rl(h1), rl(h2)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// needsSimilar checks if distance indicates truly similar colors (not just same luminance)
// Threshold of 20 catches genuinely similar colors without false positives from hue differences
func needsSimilar(dist float64) bool {
	return dist >= 0 && dist < 20
}

// needsIdentical catches when two colors are the same or near-identical
func needsIdentical(dist float64) bool {
	return dist >= 0 && dist < 0.1
}

// minDistBetween returns the minimum weighted distance from hex to any color in list
func minDistBetween(hex string, list []string) float64 {
	min := -1.0
	for _, other := range list {
		if other == "" {
			continue
		}
		d := weightedDist(hex, other)
		if min < 0 || (d >= 0 && d < min) {
			min = d
		}
	}
	return min
}

// findReplacement picks from theme's color1-6.
// Candidate must: not be in exclude list, and be wD > 30 from every mustDifferFrom color.
// If preferStriking, picks most vivid; otherwise picks first available.
func findReplacement(colors map[string]string, exclude, mustDifferFrom []string, preferStriking bool) string {
	var candidates []string
	for _, idx := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15"} {
		c := colors[idx]
		if c == "" || isExcluded(c, exclude) {
			continue
		}
		// Must be visually distinct from all mustDifferFrom colors
		if minDistBetween(c, mustDifferFrom) < 30 {
			continue
		}
		candidates = append(candidates, c)
	}
	if len(candidates) == 0 {
		return ""
	}
	if !preferStriking {
		return candidates[0]
	}
	best, bestScore := "", -1.0
	for _, c := range candidates {
		if s := strikingScore(c); s > bestScore {
			best, bestScore = c, s
		}
	}
	return best
}

// strikingScore returns 0-1: higher = more vivid/bright
func strikingScore(hex string) float64 {
	r, g, b, ok := parseHex(hex)
	if !ok {
		return 0
	}
	maxC := float64(max(r, g, b)) / 255.0
	minC := float64(min(r, g, b)) / 255.0
	var sat float64
	if maxC > 0 {
		sat = (maxC - minC) / maxC
	}
	return maxC * sat
}

func isExcluded(hex string, exclude []string) bool {
	for _, e := range exclude {
		if strings.EqualFold(hex, e) {
			return true
		}
	}
	return false
}

func saturation(r, g, b int) float64 {
	maxC := float64(max(r, g, b))
	minC := float64(min(r, g, b))
	if maxC == 0 {
		return 0
	}
	return (maxC - minC) / maxC
}

func clamp(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

func toHex(r, g, b int) string {
	return fmt.Sprintf("#%02x%02x%02x", clamp(r), clamp(g), clamp(b))
}

// desaturateToTarget desaturates a hex color to approximately targetSat
// by blending each channel toward neutral gray (#808080).
// Preserves hue direction and lightness character while reducing saturation.
func desaturateToTarget(hex string, targetSat float64) string {
	r, g, b, ok := parseHex(hex)
	if !ok {
		return hex
	}
	sat := saturation(r, g, b)
	if sat <= targetSat {
		return hex
	}

	maxC := float64(max(r, g, b))
	minC := float64(min(r, g, b))

	// Solve for blend factor b where new_sat ≈ targetSat
	// After blending toward #80 (128) by factor f:
	//   max' = max*(1-f) + 128*f, min' = min*(1-f) + 128*f
	//   sat' = (max'-min') / max' = (max-min)*(1-f) / (max*(1-f) + 128*f)
	denom := (maxC - minC) - targetSat*maxC + 128*targetSat
	if denom <= 0 {
		return desaturateGray(hex, 0.8)
	}
	blend := ((maxC - minC) - targetSat*maxC) / denom
	if blend < 0 {
		blend = 0
	}
	if blend > 0.9 {
		blend = 0.9
	}
	return desaturateGray(hex, blend)
}

func desaturateGray(hex string, blendFactor float64) string {
	r, g, b, ok := parseHex(hex)
	if !ok {
		return hex
	}
	nr := int(math.Round(float64(r)*(1-blendFactor) + 128*blendFactor))
	ng := int(math.Round(float64(g)*(1-blendFactor) + 128*blendFactor))
	nb := int(math.Round(float64(b)*(1-blendFactor) + 128*blendFactor))
	return toHex(nr, ng, nb)
}

// padVW pads a rendered string to target visual width
func padVW(rendered string, target int) string {
	vw := lipgloss.Width(rendered)
	if vw >= target {
		return rendered
	}
	return rendered + strings.Repeat(" ", target-vw)
}

// renderSwatch matches reference: Padding(0,2), black foreground
func renderSwatch(hex string) string {
	return lipgloss.NewStyle().
		Background(lipgloss.Color(hex)).
		Foreground(lipgloss.Color("0")).
		Padding(0, 2).
		Render("   ")
}

// renderPlaceholder matches renderSwatch width exactly
func renderPlaceholder() string {
	return lipgloss.NewStyle().
		Background(lipgloss.Color("8")).
		Foreground(lipgloss.Color("7")).
		Padding(0, 2).
		Render("n/a")
}

// renderColorLine builds one labeled swatch row, returns the rendered string
// label is 2 chars (bg, fg, ac, mu, cu), padded to 3 for slight indent
func renderColorLine(label, hex string) string {
	labelPart := padVW(" "+label, 4)
	var sw string
	if hex == "" {
		sw = renderPlaceholder()
	} else {
		sw = renderSwatch(hex)
	}
	return labelPart + sw
}

// buildThemeLines returns all lines for one theme (each ends with \n)
func buildThemeLines(t ThemeInfo) string {
	hasFix := t.FixAccent != "" || t.FixCursor != "" || t.FixMuted != ""

	var b strings.Builder

	// Theme name
	b.WriteString(t.Name + "\n")

	type row struct{ label, hex, fix string }
	rows := []row{
		{"bg", t.Background, ""},
		{"fg", t.Foreground, ""},
		{"ac", t.Accent, t.FixAccent},
		{"mu", t.Muted, t.FixMuted},
		{"cu", t.Cursor, t.FixCursor},
	}

	// Metrics string
	metrics := "  "
	if t.Foreground != "" {
		metrics = fmt.Sprintf("  ac wD=%.0f cR=%.2f  cu wD=%.0f cR=%.2f",
			t.AccentDist, t.AccentRatio, t.CursorDist, t.CursorRatio)
		if t.Muted != "" {
			metrics += fmt.Sprintf("  mu wD=%.0f sat=%.2f", t.MutedDist, t.MutedSat)
		}
	}

	for _, r := range rows {
		left := renderColorLine(r.label, r.hex)

		if hasFix {
			rightHex := r.hex
			suffix := ""
			if r.fix != "" {
				rightHex = r.fix
				suffix = " ←"
			}
			right := renderColorLine(r.label, rightHex) + suffix
			b.WriteString(left + "   " + right + "\n")
		} else {
			b.WriteString(left + metrics + "\n")
		}
	}

	b.WriteString("\n")
	return b.String()
}

func main() {
	themesDir := filepath.Join(xdg.ConfigHome, "omarchy", "themes")
	entries, err := os.ReadDir(themesDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	norm := func(s string) string {
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "0x") {
			return "#" + s[2:]
		}
		return s
	}

	var themes []ThemeInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(themesDir, e.Name(), "colors.toml"))
		if err != nil {
			continue
		}
		var tc ThemeColors
		if toml.Unmarshal(data, &tc) != nil {
			continue
		}

		info := ThemeInfo{
			Name:       e.Name(),
			Background: norm(tc.Background), Foreground: norm(tc.Foreground),
			Accent: norm(tc.Accent), Cursor: norm(tc.Cursor),
			Colors: map[string]string{
				"0": norm(tc.Color0), "1": norm(tc.Color1), "2": norm(tc.Color2),
				"3": norm(tc.Color3), "4": norm(tc.Color4), "5": norm(tc.Color5),
				"6": norm(tc.Color6), "7": norm(tc.Color7), "8": norm(tc.Color8),
				"9": norm(tc.Color9), "10": norm(tc.Color10), "11": norm(tc.Color11),
				"12": norm(tc.Color12), "13": norm(tc.Color13), "14": norm(tc.Color14),
				"15": norm(tc.Color15),
			},
		}
		// Use color7 for muted (fallback to muted field)
		if tc.Color7 != "" {
			info.Muted = norm(tc.Color7)
		} else {
			info.Muted = norm(tc.Muted)
		}

		// Compute distances
		if info.Foreground != "" && info.Accent != "" {
			info.AccentDist = weightedDist(info.Accent, info.Foreground)
			info.AccentRatio = contrastRatio(info.Accent, info.Foreground)
		}
		if info.Foreground != "" && info.Cursor != "" {
			info.CursorDist = weightedDist(info.Cursor, info.Foreground)
			info.CursorRatio = contrastRatio(info.Cursor, info.Foreground)
		}

		// --- Fallback logic (ordered) ---
		curCursor := info.Cursor
		curAccent := info.Accent
		exclude := []string{info.Foreground}

		// Step 1: Cursor vs foreground (must be visibly different)
		if info.Cursor != "" && needsSimilar(info.CursorDist) {
			if fix := findReplacement(info.Colors, exclude, []string{info.Foreground, info.Cursor, info.Accent}, false); fix != "" {
				info.FixCursor = fix
				curCursor = fix
				exclude = append(exclude, info.Cursor, fix)
			}
		}

		// Step 2: Accent vs cursor (must always differ — hard requirement for rptui)
		if info.Accent != "" && curCursor != "" && needsIdentical(weightedDist(info.Accent, curCursor)) {
			if fix := findReplacement(info.Colors, exclude, []string{info.Foreground, curCursor}, true); fix != "" {
				info.FixAccent = fix
				curAccent = fix
				exclude = append(exclude, info.Accent, fix)
			}
		}

		// Step 3: Accent vs foreground (must be visibly different)
		if info.Accent != "" && needsSimilar(info.AccentDist) {
			if curAccent == info.Accent { // not already changed in step 2
				if fix := findReplacement(info.Colors, exclude, []string{info.Foreground, curCursor}, true); fix != "" {
					info.FixAccent = fix
				}
			}
		}

		// Step 4: Muted validation (accent vs muted + saturation check)
		curMuted := info.Muted
		if info.Muted != "" {
			// Compute muted metrics
			if info.Muted != "" {
				r7, g7, b7, ok7 := parseHex(info.Muted)
				if ok7 {
					info.MutedSat = saturation(r7, g7, b7)
				}
			}
			if info.Accent != "" {
				info.MutedDist = weightedDist(info.Accent, info.Muted)
			}

			needsFix := false
			reason := ""

			if info.Accent != "" && info.MutedDist >= 0 && info.MutedDist < 30 {
				needsFix = true
				reason = "too close to accent"
			} else if info.MutedSat > 0.35 {
				needsFix = true
				reason = "too saturated"
			}

			if needsFix {
				// Try color8 first
				c8 := info.Colors["8"]
				c8ok := false
				if c8 != "" {
					r8, g8, b8, ok8 := parseHex(c8)
					if ok8 {
						c8sat := saturation(r8, g8, b8)
						if c8sat < 0.35 && (info.Accent == "" || weightedDist(info.Accent, c8) >= 30) {
							info.FixMuted = c8
							curMuted = c8
							c8ok = true
						}
					}
				}

				if !c8ok {
					// Desaturate color7 to target saturation 0.25
					desat := desaturateToTarget(info.Muted, 0.25)
					if desat != info.Muted {
						info.FixMuted = desat
						curMuted = desat
					}
				}

				_ = reason
				_ = curMuted
			}
		}
		themes = append(themes, info)
	}

	sort.Slice(themes, func(i, j int) bool { return themes[i].Name < themes[j].Name })

	for _, t := range themes {
		fmt.Print(buildThemeLines(t))
	}

	// Summary
	var fixed, accFix, curFix, muFix int
	for _, t := range themes {
		if t.FixAccent != "" {
			accFix++
		}
		if t.FixCursor != "" {
			curFix++
		}
		if t.FixMuted != "" {
			muFix++
		}
		if t.FixAccent != "" || t.FixCursor != "" || t.FixMuted != "" {
			fixed++
		}
	}
	fmt.Printf("Summary: %d themes, %d would be modified (acc:%d cur:%d mu:%d)\n",
		len(themes), fixed, accFix, curFix, muFix)
}
