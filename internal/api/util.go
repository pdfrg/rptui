package api

import (
	"regexp"
	"strings"
)

// normalizeForCompare strips "the" prefix, diacritical marks, and normalizes
// punctuation for fuzzy artist name matching across APIs.
func normalizeForCompare(s string) string {
	s = regexp.MustCompile(`^the\s+`).ReplaceAllString(s, "")
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
