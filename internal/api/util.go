package api

import (
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// normalizeForCompare strips "the" prefix, diacritical marks, and normalizes
// punctuation for fuzzy artist name matching across APIs.
func normalizeForCompare(s string) string {
	// Strip leading "the" but not if it would leave nothing useful (e.g., "The The" → "the")
	stripped := regexp.MustCompile(`^the\s+`).ReplaceAllString(s, "")
	if stripped != "" && stripped != "the" {
		s = stripped
	}
	s = strings.Map(func(r rune) rune {
		switch r {
		case '\'', '\u2018', '\u2019', '\u201A', '\u201B', '\u02BC':
			return -1
		case '-', '\u2010', '\u2011', '\u2012', '\u2013', '\u2014':
			return -1
		case '&':
			return -1
		case ',':
			return -1
		case '.':
			return -1
		case 'à', 'á', 'â', 'ã', 'ä', 'å':
			return 'a'
		case 'è', 'é', 'ê', 'ë':
			return 'e'
		case 'ì', 'í', 'î', 'ï':
			return 'i'
		case 'ò', 'ó', 'ô', 'õ', 'ö':
			return 'o'
		case 'ù', 'ú', 'û', 'ü':
			return 'u'
		case 'ý', 'ÿ':
			return 'y'
		case 'ñ':
			return 'n'
		case 'ç':
			return 'c'
		case 'ß':
			return 's'
		case 'À', 'Á', 'Â', 'Ã', 'Ä', 'Å':
			return 'A'
		case 'È', 'É', 'Ê', 'Ë':
			return 'E'
		case 'Ì', 'Í', 'Î', 'Ï':
			return 'I'
		case 'Ò', 'Ó', 'Ô', 'Õ', 'Ö':
			return 'O'
		case 'Ù', 'Ú', 'Û', 'Ü':
			return 'U'
		case 'Ñ':
			return 'N'
		case 'Ç':
			return 'C'
		}
		return r
	}, s)
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// NormalizeAlbumName normalizes an album name for fuzzy comparison.
// Strips remaster/reissue suffixes, accents, and punctuation.
func NormalizeAlbumName(s string) string {
	s = strings.ToLower(s)
	s = regexp.MustCompile(`\s*\(?(remaster(ed)?|deluxe|expanded|reissue|anniversary|bonus tracks?|special edition)\s*\d*\)?`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s*\[?(remaster(ed)?|deluxe|expanded|reissue|anniversary)\]?\s*\d*`).ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		switch r {
		case '\'', '"', ',', '.', ':', ';', '!', '?', '(', ')', '[', ']', '-', '\u2019', '\u2018':
			return -1
		case 'à', 'á', 'â', 'ã', 'ä', 'å':
			return 'a'
		case 'è', 'é', 'ê', 'ë':
			return 'e'
		case 'ì', 'í', 'î', 'ï':
			return 'i'
		case 'ò', 'ó', 'ô', 'õ', 'ö':
			return 'o'
		case 'ù', 'ú', 'û', 'ü':
			return 'u'
		case 'ý', 'ÿ':
			return 'y'
		case 'ñ':
			return 'n'
		case 'ç', 'č', 'ć':
			return 'c'
		case 'š':
			return 's'
		case 'ž':
			return 'z'
		case 'ř':
			return 'r'
		case 'ď', 'đ':
			return 'd'
		case 'ť':
			return 't'
		case 'ň':
			return 'n'
		case 'ß':
			return 's'
		}
		return r
	}, s)
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// AlbumNamesMatch checks if two album names refer to the same album using fuzzy comparison.
func AlbumNamesMatch(a, b string) bool {
	aNorm := NormalizeAlbumName(a)
	bNorm := NormalizeAlbumName(b)
	if aNorm == "" || bNorm == "" {
		return false
	}
	if aNorm == bNorm {
		return true
	}
	if strings.Contains(aNorm, bNorm) || strings.Contains(bNorm, aNorm) {
		return true
	}
	return false
}

// OpenBrowser opens a URL in the default browser (fire-and-forget).
func OpenBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	cmd.Start()
}
