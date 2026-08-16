package cmd

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/igaudit"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// captureOutErr runs fn and returns everything it wrote to stdout and stderr
// combined. Findings go to stderr and clean lines to stdout, so a test that
// only watched one stream would miss half the report.
func captureOutErr(t *testing.T, fn func()) string {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = w, w
	defer func() { os.Stdout, os.Stderr = origOut, origErr }()

	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()
	_ = w.Close()
	return <-done
}

func TestPrintIGTerminal_NoLockFile(t *testing.T) {
	var got int
	out := captureOutErr(t, func() { got = printIGTerminal(igaudit.Report{}, nil) })

	if got != 0 {
		t.Errorf("problem count = %d, want 0", got)
	}
	// A project without a lock file has not done anything wrong, so the section
	// has to read as a hint rather than as a finding.
	if !strings.Contains(out, "no fhirlint.lock found") {
		t.Errorf("want a hint about the missing lock file, got:\n%s", out)
	}
	if !strings.Contains(out, "fhirlint validate --lock") {
		t.Errorf("want the command that writes a lock file, got:\n%s", out)
	}
}

func TestPrintIGTerminal_ReadError(t *testing.T) {
	var got int
	out := captureOutErr(t, func() {
		got = printIGTerminal(igaudit.Report{}, errors.New("reading fhirlint.lock: broken"))
	})

	if got != 0 {
		t.Errorf("problem count = %d, want 0 — an unreadable lock file is not a package finding", got)
	}
	if !strings.Contains(out, "broken") {
		t.Errorf("want the read error surfaced, got:\n%s", out)
	}
}

func TestPrintIGTerminal_Classification(t *testing.T) {
	report := igaudit.Report{Packages: []igaudit.PackageReport{
		{ID: "cur.pkg#1.0.0", Name: "cur.pkg", Version: "1.0.0", Latest: "1.0.0"},
		{ID: "old.pkg#1.4.0", Name: "old.pkg", Version: "1.4.0", Latest: "1.6.0", Outdated: true},
		{ID: "dep.pkg#1.0.0", Name: "dep.pkg", Version: "1.0.0", Deprecated: true, DeprecationNote: "use new.pkg"},
		{ID: "gone.pkg#1.0.0", Name: "gone.pkg", Version: "1.0.0", NotFound: true},
		{ID: "odd.pkg#1.0.0", Name: "odd.pkg", Version: "1.0.0", Latest: "2025-Q1", Differs: true},
		{ID: "new.pkg#2.0.0", Name: "new.pkg", Version: "2.0.0", Latest: "1.0.0", Ahead: true},
		{ID: "err.pkg#1.0.0", Name: "err.pkg", Version: "1.0.0", Error: "connection refused"},
	}}

	var got int
	out := captureOutErr(t, func() { got = printIGTerminal(report, nil) })

	// Outdated, deprecated, not-found and differs count. Ahead and a check that
	// could not run do not.
	if want := 4; got != want {
		t.Errorf("problem count = %d, want %d", got, want)
	}

	for _, want := range []string{
		"1.4.0 → 1.6.0 available",
		"deprecated upstream: use new.pkg",
		"not found in the registry",
		"registry latest is 2025-Q1",
		"ahead of registry latest",
		"could not check: connection refused",
		"(4 of 7 package(s) need attention)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestAuditExitCode(t *testing.T) {
	cases := []struct {
		name          string
		jar, ig, want int
	}{
		{"clean", 0, 0, 0},
		{"jar only", 1, 0, exitJARIssue},
		{"ig only", 0, 2, exitIGIssue},
		// An outdated IG package must never mask a JAR advisory: a script that
		// reacts to exit 1 alone still has to see the security-relevant case.
		{"both", 1, 2, exitJARIssue},
	}
	for _, tc := range cases {
		if got := auditExitCode(tc.jar, tc.ig); got != tc.want {
			t.Errorf("%s: auditExitCode(%d, %d) = %d, want %d", tc.name, tc.jar, tc.ig, got, tc.want)
		}
	}
}

func TestPrintAuditJSON_IGFields(t *testing.T) {
	igReport := igaudit.Report{Packages: []igaudit.PackageReport{
		{ID: "old.pkg#1.4.0", Name: "old.pkg", Version: "1.4.0", Latest: "1.6.0", Outdated: true},
	}}

	out := captureOutErr(t, func() {
		if err := printAuditJSON(validator.AuditReport{CurrentVersion: "6.10.2"}, igReport, nil); err != nil {
			t.Fatal(err)
		}
	})

	var got auditJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}

	if got.LockFile != "fhirlint.lock" {
		t.Errorf("lockFile = %q, want fhirlint.lock", got.LockFile)
	}
	if len(got.IGPackages) != 1 || !got.IGPackages[0].Outdated {
		t.Errorf("igPackages = %+v, want one outdated entry", got.IGPackages)
	}
	// The jar-monitor workflow reads these, so adding the IG section must not
	// disturb them.
	if got.CurrentVersion != "6.10.2" {
		t.Errorf("currentVersion = %q, want 6.10.2", got.CurrentVersion)
	}
	if got.Affecting == nil {
		t.Error("affecting must stay a non-nil array")
	}
}

func TestPrintAuditJSON_NoLockFile(t *testing.T) {
	out := captureOutErr(t, func() {
		if err := printAuditJSON(validator.AuditReport{CurrentVersion: "6.10.2"}, igaudit.Report{}, nil); err != nil {
			t.Fatal(err)
		}
	})

	// Consumers should be able to range over igPackages without a nil check,
	// and lockFile is omitted rather than reported as an empty path.
	if !strings.Contains(out, `"igPackages": []`) {
		t.Errorf("want an empty igPackages array, got:\n%s", out)
	}
	if strings.Contains(out, `"lockFile"`) {
		t.Errorf("lockFile must be omitted when there is no lock file, got:\n%s", out)
	}
}
