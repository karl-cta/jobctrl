package handlers

import (
	"database/sql/driver"
	"strings"

	"modernc.org/sqlite"
)

// jc_fold(text) is foldText as an SQLite function, so that search, duplicate
// detection and the A-Z sort can fold stored values on the fly: SQLite's own
// LOWER() and LIKE only fold ASCII, which misses "É". Functions registered on
// the driver apply to every connection opened afterwards, and init() runs
// before main opens the database.
func init() {
	sqlite.MustRegisterDeterministicScalarFunction("jc_fold", 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			switch v := args[0].(type) {
			case string:
				return foldText(v), nil
			case []byte:
				return foldText(string(v)), nil
			}
			return args[0], nil
		})
}

// foldText lowercases s, strips the accents of Latin letters and collapses
// runs of spaces (non-breaking ones included, as pasted from a job posting),
// so that "Société  Générale", "société générale" and "SOCIETE GENERALE" all
// fold to the same text. The table covers the letters of the languages the
// app's users write in, not all of Unicode.
func foldText(s string) string {
	s = strings.Join(strings.Fields(strings.ToLower(s)), " ")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case 'à', 'á', 'â', 'ã', 'ä', 'å':
			b.WriteByte('a')
		case 'ç':
			b.WriteByte('c')
		case 'è', 'é', 'ê', 'ë':
			b.WriteByte('e')
		case 'ì', 'í', 'î', 'ï':
			b.WriteByte('i')
		case 'ñ':
			b.WriteByte('n')
		case 'ò', 'ó', 'ô', 'õ', 'ö', 'ø':
			b.WriteByte('o')
		case 'ù', 'ú', 'û', 'ü':
			b.WriteByte('u')
		case 'ý', 'ÿ':
			b.WriteByte('y')
		case 'œ':
			b.WriteString("oe")
		case 'æ':
			b.WriteString("ae")
		case 'ß':
			b.WriteString("ss")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
