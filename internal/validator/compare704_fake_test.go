// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Each way a comparison can fail ends in its own message, so `fhirlint
// compare` says what to do rather than "no output" (#704).
func TestFakeRunCompare_Failures(t *testing.T) {
	opts, _ := fakeSetup(t)
	base := CompareOptions{FHIRVersion: "4.0.1", JARPath: opts.JARPath, IGs: []string{"kbv.basis#1.5.0"}}
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, left, want string
		edit             func(*CompareOptions)
	}{
		{"destination below a file", "http://x/A", "creating output directory", func(o *CompareOptions) { o.DestDir = filepath.Join(file, "out") }},
		{"no JAR", "http://x/A", "JAR not found at", func(o *CompareOptions) { o.JARPath = filepath.Join(t.TempDir(), "absent", "validator_cli.jar") }},
		{"out of memory", "http://x/oom", "out of memory", nil},
		{"timeout", "http://x/slow", "comparison timed out after", func(o *CompareOptions) { o.Timeout = 200 * time.Millisecond }},
		{"exit 1 and nothing said", "http://x/silent", "comparison failed: exit status 1", nil},
		{"exit 0 and no page", "http://x/empty", "comparison produced no output — check that both profiles resolve", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := base
			o.DestDir = t.TempDir()
			if c.edit != nil {
				c.edit(&o)
			}
			res, err := RunCompare(c.left, "http://x/B", o)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v (result %+v), want %q", err, res, c.want)
			}
		})
	}
}

// A data row whose severity cell is empty is not a difference: without the
// skip it would come back as a message with no severity (#704).
func TestParseCompareMessages_SkipsRowsWithoutSeverity(t *testing.T) {
	rows := `<tr><td></td><td>Patient</td><td>section heading</td></tr>` +
		`<tr><td>Warning</td><td>Patient.name</td><td>differs</td></tr>`
	msgs := parseCompareMessages(messagesHTML(rows))
	if len(msgs) != 1 || msgs[0].Severity != "warning" || msgs[0].Path != "Patient.name" {
		t.Errorf("messages = %+v, want only the warning row", msgs)
	}
}
