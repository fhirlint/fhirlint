package fhirpath

import (
	"fmt"
	"strings"
	"unicode"
)

// tokKind enumerates the token types the lexer produces.
type tokKind int

const (
	tEOF tokKind = iota
	tIdent
	tString
	tNumber
	tLParen
	tRParen
	tLBracket
	tRBracket
	tComma
	tDot
	tEq  // =
	tNeq // !=
	tLt  // <
	tGt  // >
	tLe  // <=
	tGe  // >=
)

type token struct {
	kind tokKind
	text string
	pos  int
}

// lex turns a FHIRPath expression into tokens. It supports the subset the lint
// engine implements; unsupported operator characters (e.g. '~', '&', '|', '+')
// produce an error rather than being silently ignored, so an unsupported rule
// fails to compile instead of evaluating wrongly.
func lex(input string) ([]token, error) {
	var toks []token
	runes := []rune(input)
	i := 0
	for i < len(runes) {
		c := runes[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '(':
			toks = append(toks, token{tLParen, "(", i})
			i++
		case c == ')':
			toks = append(toks, token{tRParen, ")", i})
			i++
		case c == '[':
			toks = append(toks, token{tLBracket, "[", i})
			i++
		case c == ']':
			toks = append(toks, token{tRBracket, "]", i})
			i++
		case c == ',':
			toks = append(toks, token{tComma, ",", i})
			i++
		case c == '.':
			toks = append(toks, token{tDot, ".", i})
			i++
		case c == '=':
			toks = append(toks, token{tEq, "=", i})
			i++
		case c == '!':
			if i+1 < len(runes) && runes[i+1] == '=' {
				toks = append(toks, token{tNeq, "!=", i})
				i += 2
			} else {
				return nil, fmt.Errorf("unexpected '!' at position %d — use the not() function", i)
			}
		case c == '<':
			if i+1 < len(runes) && runes[i+1] == '=' {
				toks = append(toks, token{tLe, "<=", i})
				i += 2
			} else {
				toks = append(toks, token{tLt, "<", i})
				i++
			}
		case c == '>':
			if i+1 < len(runes) && runes[i+1] == '=' {
				toks = append(toks, token{tGe, ">=", i})
				i += 2
			} else {
				toks = append(toks, token{tGt, ">", i})
				i++
			}
		case c == '\'':
			s, n, err := lexString(runes, i)
			if err != nil {
				return nil, err
			}
			toks = append(toks, token{tString, s, i})
			i = n
		case unicode.IsDigit(c):
			s, n := lexNumber(runes, i)
			toks = append(toks, token{tNumber, s, i})
			i = n
		case c == '$' || c == '_' || unicode.IsLetter(c):
			s, n := lexIdent(runes, i)
			toks = append(toks, token{tIdent, s, i})
			i = n
		default:
			return nil, fmt.Errorf("unsupported character %q at position %d", string(c), i)
		}
	}
	toks = append(toks, token{tEOF, "", len(runes)})
	return toks, nil
}

// lexString reads a single-quoted string literal starting at start (the opening
// quote) and returns the unquoted, unescaped value and the index after the
// closing quote.
func lexString(runes []rune, start int) (string, int, error) {
	var b strings.Builder
	i := start + 1
	for i < len(runes) {
		c := runes[i]
		switch c {
		case '\'':
			return b.String(), i + 1, nil
		case '\\':
			if i+1 >= len(runes) {
				return "", 0, fmt.Errorf("unterminated escape in string at position %d", i)
			}
			esc := runes[i+1]
			switch esc {
			case '\'', '"', '\\', '/', '`':
				b.WriteRune(esc)
			case 'n':
				b.WriteRune('\n')
			case 'r':
				b.WriteRune('\r')
			case 't':
				b.WriteRune('\t')
			case 'f':
				b.WriteRune('\f')
			default:
				return "", 0, fmt.Errorf("unsupported escape \\%s at position %d", string(esc), i)
			}
			i += 2
		default:
			b.WriteRune(c)
			i++
		}
	}
	return "", 0, fmt.Errorf("unterminated string literal starting at position %d", start)
}

// lexNumber reads an integer or decimal number and returns its text and the
// index after it.
func lexNumber(runes []rune, start int) (string, int) {
	i := start
	seenDot := false
	for i < len(runes) {
		c := runes[i]
		if unicode.IsDigit(c) {
			i++
			continue
		}
		if c == '.' && !seenDot && i+1 < len(runes) && unicode.IsDigit(runes[i+1]) {
			seenDot = true
			i++
			continue
		}
		break
	}
	return string(runes[start:i]), i
}

// lexIdent reads an identifier ($this, function names, path steps) and returns
// its text and the index after it.
func lexIdent(runes []rune, start int) (string, int) {
	i := start
	if runes[i] == '$' {
		i++
	}
	for i < len(runes) {
		c := runes[i]
		if c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c) {
			i++
			continue
		}
		break
	}
	return string(runes[start:i]), i
}
