package reporter

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func TestSlowest(t *testing.T) {
	results := []*validator.Result{
		{Label: "fast.json", DurationMs: 120},
		{Label: "slow.json", DurationMs: 4302},
		{Label: "cached.json", Cached: true},
		{Label: "mid\x1b[2J.json", DurationMs: 590},
	}
	var buf bytes.Buffer
	Slowest(&buf, results, 2)
	out := buf.String()

	slow, mid := strings.Index(out, "slow.json"), strings.Index(out, `mid\x1b[2J.json`)
	if slow < 0 || mid < 0 || slow > mid {
		t.Errorf("want slow.json before the escaped mid file:\n%q", out)
	}
	if strings.Contains(out, "fast.json") {
		t.Error("listed more than n files")
	}
	if !strings.Contains(out, "4.302 s") {
		t.Errorf("time not shown:\n%s", out)
	}
	if !strings.Contains(out, "1 file(s) without a time") {
		t.Errorf("cached file not accounted for:\n%s", out)
	}
}

func TestSlowest_OffOrNoTimes(t *testing.T) {
	var buf bytes.Buffer
	Slowest(&buf, []*validator.Result{{Label: "a", DurationMs: 5}}, 0)
	if buf.Len() != 0 {
		t.Errorf("--slowest 0 printed %q", buf.String())
	}
	Slowest(&buf, []*validator.Result{{Label: "a", Cached: true}}, 3)
	if !strings.Contains(buf.String(), "No validation times") {
		t.Errorf("got %q", buf.String())
	}
}
