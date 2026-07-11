package fhirpath

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

// value is a single FHIRPath item: bool, float64, string, a gjson.Result (for
// objects), or nil (JSON null). Collections are represented as []value.
type value interface{}

// eval evaluates n against the current focus collection. this is the value of
// $this (the current item inside a where()/all()/exists() criterion).
func eval(n node, focus, this []value) ([]value, error) {
	switch t := n.(type) {
	case boolLit:
		return []value{t.v}, nil
	case numLit:
		return []value{t.v}, nil
	case strLit:
		return []value{t.v}, nil
	case thisRef:
		return this, nil
	case ident:
		return navigate(focus, t.name), nil
	case indexNode:
		base, err := eval(t.base, focus, this)
		if err != nil {
			return nil, err
		}
		if t.idx < 0 || t.idx >= len(base) {
			return nil, nil
		}
		return []value{base[t.idx]}, nil
	case invoke:
		mid, err := eval(t.base, focus, this)
		if err != nil {
			return nil, err
		}
		return evalStep(t.step, mid, this)
	case call:
		return evalCall(t, focus, this)
	case binop:
		return evalBinop(t, focus, this)
	case logic:
		return evalLogic(t, focus, this)
	default:
		return nil, fmt.Errorf("internal error: unhandled node %T", n)
	}
}

// evalStep applies the part after a '.' (a path step or a function) to mid.
func evalStep(step node, mid, this []value) ([]value, error) {
	switch s := step.(type) {
	case ident:
		return navigate(mid, s.name), nil
	case call:
		return evalCall(s, mid, this)
	default:
		return nil, fmt.Errorf("internal error: unexpected step %T", step)
	}
}

// navigate returns the children named name across every object in focus. As a
// special case, navigating a resource object by its own resourceType returns the
// object itself, so "Patient.name" and a bare "Patient" behave as a type filter.
func navigate(focus []value, name string) []value {
	var out []value
	for _, v := range focus {
		r, ok := v.(gjson.Result)
		if !ok || !r.IsObject() {
			continue
		}
		if child := r.Get(name); child.Exists() {
			out = append(out, expand(child)...)
		} else if r.Get("resourceType").String() == name {
			out = append(out, r)
		}
	}
	return out
}

// expand flattens a gjson value into items: arrays become one item per element,
// everything else a single item.
func expand(r gjson.Result) []value {
	if r.IsArray() {
		var out []value
		r.ForEach(func(_, e gjson.Result) bool {
			out = append(out, leaf(e))
			return true
		})
		return out
	}
	return []value{leaf(r)}
}

// leaf converts a scalar gjson value to a native value, keeping objects as
// gjson.Result for further navigation.
func leaf(r gjson.Result) value {
	switch r.Type {
	case gjson.True:
		return true
	case gjson.False:
		return false
	case gjson.String:
		return r.Str
	case gjson.Number:
		return r.Num
	case gjson.Null:
		return nil
	default:
		return r
	}
}

func evalCall(c call, focus, this []value) ([]value, error) {
	switch c.name {
	case "exists":
		if err := wantArgs(c, 0, 1); err != nil {
			return nil, err
		}
		if len(c.args) == 1 {
			filtered, err := filter(focus, c.args[0])
			if err != nil {
				return nil, err
			}
			return []value{len(filtered) > 0}, nil
		}
		return []value{len(focus) > 0}, nil
	case "empty":
		if err := wantArgs(c, 0, 0); err != nil {
			return nil, err
		}
		return []value{len(focus) == 0}, nil
	case "count":
		if err := wantArgs(c, 0, 0); err != nil {
			return nil, err
		}
		return []value{float64(len(focus))}, nil
	case "where":
		if err := wantArgs(c, 1, 1); err != nil {
			return nil, err
		}
		return filter(focus, c.args[0])
	case "all":
		if err := wantArgs(c, 1, 1); err != nil {
			return nil, err
		}
		for _, e := range focus {
			ok, err := criterion(c.args[0], e)
			if err != nil {
				return nil, err
			}
			if !ok {
				return []value{false}, nil
			}
		}
		return []value{true}, nil
	case "first":
		if err := wantArgs(c, 0, 0); err != nil {
			return nil, err
		}
		if len(focus) == 0 {
			return nil, nil
		}
		return []value{focus[0]}, nil
	case "last":
		if err := wantArgs(c, 0, 0); err != nil {
			return nil, err
		}
		if len(focus) == 0 {
			return nil, nil
		}
		return []value{focus[len(focus)-1]}, nil
	case "not":
		if err := wantArgs(c, 0, 0); err != nil {
			return nil, err
		}
		if len(focus) == 0 {
			return nil, nil
		}
		b, ok := single[bool](focus)
		if !ok {
			return nil, fmt.Errorf("not() expects a single boolean")
		}
		return []value{!b}, nil
	case "hasValue":
		if err := wantArgs(c, 0, 0); err != nil {
			return nil, err
		}
		return []value{isSinglePrimitive(focus)}, nil
	case "length":
		if err := wantArgs(c, 0, 0); err != nil {
			return nil, err
		}
		s, ok, err := stringFocus(focus)
		if err != nil || !ok {
			return emptyOr(err)
		}
		return []value{float64(utf8.RuneCountInString(s))}, nil
	case "toString":
		if err := wantArgs(c, 0, 0); err != nil {
			return nil, err
		}
		if len(focus) == 0 {
			return nil, nil
		}
		if len(focus) > 1 {
			return nil, fmt.Errorf("toString() expects a single item")
		}
		return []value{formatScalar(focus[0])}, nil
	case "startsWith", "endsWith", "contains", "matches":
		return evalStringFunc(c, focus, this)
	default:
		return nil, fmt.Errorf("unsupported function %q", c.name)
	}
}

// evalStringFunc implements the single-string-argument string functions.
func evalStringFunc(c call, focus, this []value) ([]value, error) {
	if err := wantArgs(c, 1, 1); err != nil {
		return nil, err
	}
	s, ok, err := stringFocus(focus)
	if err != nil || !ok {
		return emptyOr(err)
	}
	argVals, err := eval(c.args[0], focus, this)
	if err != nil {
		return nil, err
	}
	arg, ok := single[string](argVals)
	if !ok {
		return nil, fmt.Errorf("%s() expects a single string argument", c.name)
	}
	switch c.name {
	case "startsWith":
		return []value{strings.HasPrefix(s, arg)}, nil
	case "endsWith":
		return []value{strings.HasSuffix(s, arg)}, nil
	case "contains":
		return []value{strings.Contains(s, arg)}, nil
	case "matches":
		re, err := compileRegex(arg)
		if err != nil {
			return nil, fmt.Errorf("matches(): %w", err)
		}
		return []value{re.MatchString(s)}, nil
	}
	return nil, fmt.Errorf("internal error: string func %q", c.name)
}

func evalBinop(b binop, focus, this []value) ([]value, error) {
	lv, err := eval(b.l, focus, this)
	if err != nil {
		return nil, err
	}
	rv, err := eval(b.r, focus, this)
	if err != nil {
		return nil, err
	}
	// Comparisons and equality on an empty operand yield empty per FHIRPath.
	if len(lv) == 0 || len(rv) == 0 {
		return nil, nil
	}
	if len(lv) != 1 || len(rv) != 1 {
		return nil, fmt.Errorf("comparison requires single-item operands")
	}
	switch b.op {
	case tEq, tNeq:
		eq, err := equalValues(lv[0], rv[0])
		if err != nil {
			return nil, err
		}
		if b.op == tNeq {
			eq = !eq
		}
		return []value{eq}, nil
	case tLt, tGt, tLe, tGe:
		cmp, err := compareValues(lv[0], rv[0])
		if err != nil {
			return nil, err
		}
		switch b.op {
		case tLt:
			return []value{cmp < 0}, nil
		case tGt:
			return []value{cmp > 0}, nil
		case tLe:
			return []value{cmp <= 0}, nil
		default:
			return []value{cmp >= 0}, nil
		}
	}
	return nil, fmt.Errorf("internal error: binop %d", b.op)
}

func evalLogic(l logic, focus, this []value) ([]value, error) {
	if l.op == "in" {
		return evalMembership(l, focus, this)
	}
	lv, err := eval(l.l, focus, this)
	if err != nil {
		return nil, err
	}
	rv, err := eval(l.r, focus, this)
	if err != nil {
		return nil, err
	}
	lb := truthy3(lv)
	rb := truthy3(rv)
	var res *bool
	switch l.op {
	case "and":
		res = logicAnd(lb, rb)
	case "or":
		res = logicOr(lb, rb)
	case "xor":
		if lb != nil && rb != nil {
			v := *lb != *rb
			res = &v
		}
	case "implies":
		res = logicImplies(lb, rb)
	default:
		return nil, fmt.Errorf("internal error: logic %q", l.op)
	}
	if res == nil {
		return nil, nil
	}
	return []value{*res}, nil
}

func evalMembership(l logic, focus, this []value) ([]value, error) {
	lv, err := eval(l.l, focus, this)
	if err != nil {
		return nil, err
	}
	rv, err := eval(l.r, focus, this)
	if err != nil {
		return nil, err
	}
	if len(lv) == 0 {
		return nil, nil
	}
	if len(lv) != 1 {
		return nil, fmt.Errorf("'in' requires a single-item left operand")
	}
	for _, cand := range rv {
		eq, err := equalValues(lv[0], cand)
		if err != nil {
			return nil, err
		}
		if eq {
			return []value{true}, nil
		}
	}
	return []value{false}, nil
}

// filter keeps the items of focus for which crit is truthy.
func filter(focus []value, crit node) ([]value, error) {
	var out []value
	for _, e := range focus {
		ok, err := criterion(crit, e)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, e)
		}
	}
	return out, nil
}

// criterion evaluates a where()/all()/exists() criterion against a single item,
// with that item as both the focus and $this, and interprets the result as a
// boolean: empty or [false] is false, [true] is true, and any other non-empty
// result is treated as present (true).
func criterion(crit node, item value) (bool, error) {
	res, err := eval(crit, []value{item}, []value{item})
	if err != nil {
		return false, err
	}
	b := truthy3(res)
	return b != nil && *b, nil
}

// truthy3 maps a collection to a three-valued boolean: nil means empty/unknown.
// [false] is false, [true] is true, any other non-empty collection is true.
func truthy3(vals []value) *bool {
	if len(vals) == 0 {
		return nil
	}
	if len(vals) == 1 {
		if b, ok := vals[0].(bool); ok {
			return &b
		}
	}
	t := true
	return &t
}

func logicAnd(l, r *bool) *bool {
	if isFalse(l) || isFalse(r) {
		f := false
		return &f
	}
	if isTrue(l) && isTrue(r) {
		t := true
		return &t
	}
	return nil
}

func logicOr(l, r *bool) *bool {
	if isTrue(l) || isTrue(r) {
		t := true
		return &t
	}
	if isFalse(l) && isFalse(r) {
		f := false
		return &f
	}
	return nil
}

func logicImplies(l, r *bool) *bool {
	if isFalse(l) {
		t := true
		return &t
	}
	if isTrue(l) {
		return r
	}
	if isTrue(r) { // l unknown
		t := true
		return &t
	}
	return nil
}

func isTrue(b *bool) bool  { return b != nil && *b }
func isFalse(b *bool) bool { return b != nil && !*b }

// equalValues compares two scalar values for FHIRPath equality.
func equalValues(a, b value) (bool, error) {
	switch av := a.(type) {
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv, nil
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv, nil
	case string:
		bv, ok := b.(string)
		return ok && av == bv, nil
	case gjson.Result:
		return false, fmt.Errorf("cannot compare complex (object) values")
	case nil:
		return b == nil, nil
	default:
		return false, fmt.Errorf("cannot compare value of type %T", a)
	}
}

// compareValues orders two scalar values, returning -1, 0 or 1.
func compareValues(a, b value) (int, error) {
	switch av := a.(type) {
	case float64:
		bv, ok := b.(float64)
		if !ok {
			return 0, fmt.Errorf("cannot compare number with %s", typeName(b))
		}
		switch {
		case av < bv:
			return -1, nil
		case av > bv:
			return 1, nil
		default:
			return 0, nil
		}
	case string:
		bv, ok := b.(string)
		if !ok {
			return 0, fmt.Errorf("cannot compare string with %s", typeName(b))
		}
		return strings.Compare(av, bv), nil
	default:
		return 0, fmt.Errorf("cannot order value of type %s", typeName(a))
	}
}

// stringFocus returns the single string value of focus. ok is false (with no
// error) when focus is empty, so string functions on a missing element yield an
// empty result rather than failing.
func stringFocus(focus []value) (string, bool, error) {
	if len(focus) == 0 {
		return "", false, nil
	}
	if len(focus) > 1 {
		return "", false, fmt.Errorf("expected a single string, got %d items", len(focus))
	}
	s, ok := focus[0].(string)
	if !ok {
		return "", false, fmt.Errorf("expected a string, got %s", typeName(focus[0]))
	}
	return s, true, nil
}

// single returns the sole value of vals if it has exactly one item of type T.
func single[T any](vals []value) (T, bool) {
	var zero T
	if len(vals) != 1 {
		return zero, false
	}
	v, ok := vals[0].(T)
	return v, ok
}

func isSinglePrimitive(focus []value) bool {
	if len(focus) != 1 {
		return false
	}
	switch focus[0].(type) {
	case bool, float64, string:
		return true
	default:
		return false
	}
}

// emptyOr returns an empty result when err is nil, or the error otherwise. It
// lets string/length functions treat a missing (empty) focus as an empty result.
func emptyOr(err error) ([]value, error) {
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func wantArgs(c call, min, max int) error {
	if len(c.args) < min || len(c.args) > max {
		if min == max {
			return fmt.Errorf("%s() expects %d argument(s), got %d", c.name, min, len(c.args))
		}
		return fmt.Errorf("%s() expects %d to %d arguments, got %d", c.name, min, max, len(c.args))
	}
	return nil
}

func formatScalar(v value) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

func typeName(v value) string {
	switch v.(type) {
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case gjson.Result:
		return "object"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", v)
	}
}

var regexCache sync.Map // pattern -> *regexp.Regexp

func compileRegex(pattern string) (*regexp.Regexp, error) {
	if v, ok := regexCache.Load(pattern); ok {
		return v.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	regexCache.Store(pattern, re)
	return re, nil
}
