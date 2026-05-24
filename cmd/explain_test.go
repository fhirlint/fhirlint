package cmd

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn and returns everything it wrote to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestRunExplain_KnownID(t *testing.T) {
	flagExplainList = false
	var err error
	out := captureStdout(t, func() { err = runExplain(nil, []string{"dom-6"}) })
	if err != nil {
		t.Fatalf("expected nil error for known ID, got %v", err)
	}
	if !strings.Contains(out, "dom-6 — A resource should have narrative") {
		t.Errorf("expected explanation output, got:\n%s", out)
	}
}

func TestRunExplain_CaseInsensitive(t *testing.T) {
	flagExplainList = false
	var err error
	_ = captureStdout(t, func() { err = runExplain(nil, []string{"DOM-6"}) })
	if err != nil {
		t.Errorf("expected nil error for upper-case ID, got %v", err)
	}
}

func TestRunExplain_UnknownID(t *testing.T) {
	flagExplainList = false
	err := runExplain(nil, []string{"made-up-99"})
	if err == nil {
		t.Fatal("expected error for unknown ID")
	}
	if !strings.Contains(err.Error(), "hl7.org/fhir") {
		t.Errorf("expected spec-lookup suggestion in error, got %q", err.Error())
	}
}

func TestRunExplain_NoArgsWithoutList(t *testing.T) {
	flagExplainList = false
	if err := runExplain(nil, nil); err == nil {
		t.Error("expected error when no ID given and --list not set")
	}
}

func TestRunExplain_List(t *testing.T) {
	flagExplainList = true
	defer func() { flagExplainList = false }()
	var err error
	out := captureStdout(t, func() { err = runExplain(nil, nil) })
	if err != nil {
		t.Fatalf("expected nil error for --list, got %v", err)
	}
	if !strings.Contains(out, "dom-6") || !strings.Contains(out, "ele-1") {
		t.Errorf("expected --list to include known IDs, got:\n%s", out)
	}
}
