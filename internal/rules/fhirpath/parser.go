package fhirpath

import (
	"fmt"
	"strconv"
)

// node is an AST node of a parsed FHIRPath expression.
type node interface{}

type boolLit struct{ v bool }
type numLit struct{ v float64 }
type strLit struct{ v string }
type thisRef struct{}
type ident struct{ name string } // a path step evaluated against the current focus
type call struct {
	name string
	args []node
} // a function applied to the current focus
type indexNode struct {
	base node
	idx  int
}
type invoke struct{ base, step node } // base '.' step, where step is an ident or call
type binop struct {
	op   tokKind
	l, r node
} // =, !=, <, >, <=, >=
type logic struct {
	op   string
	l, r node
} // and, or, xor, implies, in

// Parse turns a FHIRPath expression into an AST, returning an error for syntax
// the lint subset does not support.
func Parse(input string) (node, error) {
	toks, err := lex(input)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	n, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tEOF {
		return nil, fmt.Errorf("unexpected %q at position %d", p.peek().text, p.peek().pos)
	}
	return n, nil
}

type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }

func (p *parser) isKeyword(kw string) bool {
	t := p.peek()
	return t.kind == tIdent && t.text == kw
}

func (p *parser) parseExpr() (node, error) { return p.parseImplies() }

func (p *parser) parseImplies() (node, error) {
	l, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("implies") {
		p.next()
		r, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		l = logic{op: "implies", l: l, r: r}
	}
	return l, nil
}

func (p *parser) parseOr() (node, error) {
	l, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("or") || p.isKeyword("xor") {
		op := p.next().text
		r, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		l = logic{op: op, l: l, r: r}
	}
	return l, nil
}

func (p *parser) parseAnd() (node, error) {
	l, err := p.parseMembership()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("and") {
		p.next()
		r, err := p.parseMembership()
		if err != nil {
			return nil, err
		}
		l = logic{op: "and", l: l, r: r}
	}
	return l, nil
}

func (p *parser) parseMembership() (node, error) {
	l, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("in") {
		p.next()
		r, err := p.parseEquality()
		if err != nil {
			return nil, err
		}
		l = logic{op: "in", l: l, r: r}
	}
	return l, nil
}

func (p *parser) parseEquality() (node, error) {
	l, err := p.parseComparison()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tEq || p.peek().kind == tNeq {
		op := p.next().kind
		r, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		l = binop{op: op, l: l, r: r}
	}
	return l, nil
}

func (p *parser) parseComparison() (node, error) {
	l, err := p.parsePostfix()
	if err != nil {
		return nil, err
	}
	for k := p.peek().kind; k == tLt || k == tGt || k == tLe || k == tGe; k = p.peek().kind {
		op := p.next().kind
		r, err := p.parsePostfix()
		if err != nil {
			return nil, err
		}
		l = binop{op: op, l: l, r: r}
	}
	return l, nil
}

// parsePostfix parses a primary term followed by any chain of '.'-invocations
// and '[index]' selectors.
func (p *parser) parsePostfix() (node, error) {
	base, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		switch p.peek().kind {
		case tDot:
			p.next()
			step, err := p.parseInvocation()
			if err != nil {
				return nil, err
			}
			base = invoke{base: base, step: step}
		case tLBracket:
			p.next()
			if p.peek().kind != tNumber {
				return nil, fmt.Errorf("expected integer index at position %d", p.peek().pos)
			}
			numTok := p.next()
			idx, err := strconv.Atoi(numTok.text)
			if err != nil {
				return nil, fmt.Errorf("invalid index %q at position %d", numTok.text, numTok.pos)
			}
			if p.peek().kind != tRBracket {
				return nil, fmt.Errorf("expected ']' at position %d", p.peek().pos)
			}
			p.next()
			base = indexNode{base: base, idx: idx}
		default:
			return base, nil
		}
	}
}

// parseInvocation parses the part after a '.': either a function call or a path step.
func (p *parser) parseInvocation() (node, error) {
	if p.peek().kind != tIdent {
		return nil, fmt.Errorf("expected identifier after '.' at position %d", p.peek().pos)
	}
	name := p.next().text
	if p.peek().kind == tLParen {
		return p.parseCallArgs(name)
	}
	return ident{name: name}, nil
}

// parsePrimary parses a literal, $this, a parenthesised expression, a function
// call, or a leading path step.
func (p *parser) parsePrimary() (node, error) {
	t := p.peek()
	switch t.kind {
	case tLParen:
		p.next()
		n, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tRParen {
			return nil, fmt.Errorf("expected ')' at position %d", p.peek().pos)
		}
		p.next()
		return n, nil
	case tString:
		p.next()
		return strLit{v: t.text}, nil
	case tNumber:
		p.next()
		f, err := strconv.ParseFloat(t.text, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q at position %d", t.text, t.pos)
		}
		return numLit{v: f}, nil
	case tIdent:
		p.next()
		switch t.text {
		case "true":
			return boolLit{v: true}, nil
		case "false":
			return boolLit{v: false}, nil
		case "$this":
			return thisRef{}, nil
		}
		if p.peek().kind == tLParen {
			return p.parseCallArgs(t.text)
		}
		return ident{name: t.text}, nil
	default:
		return nil, fmt.Errorf("unexpected %q at position %d", t.text, t.pos)
	}
}

// parseCallArgs parses "( arg (',' arg)* )" for a function named name, with the
// opening paren as the current token.
func (p *parser) parseCallArgs(name string) (node, error) {
	p.next() // consume '('
	var args []node
	if p.peek().kind != tRParen {
		for {
			arg, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			if p.peek().kind == tComma {
				p.next()
				continue
			}
			break
		}
	}
	if p.peek().kind != tRParen {
		return nil, fmt.Errorf("expected ')' to close %s( at position %d", name, p.peek().pos)
	}
	p.next() // consume ')'
	return call{name: name, args: args}, nil
}
