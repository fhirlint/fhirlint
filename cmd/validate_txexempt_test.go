package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// applyInsecureTxExemption is the narrow lever from #386: it exists only for a
// plain-HTTP terminology server the user asked for by name.
func TestApplyInsecureTxExemption_WritesAOneURLExemption(t *testing.T) {
	opts := validator.Options{
		TerminologyServer: "http://127.0.0.1:8080/fhir",
		AllowInsecureTx:   true,
	}
	var w bytes.Buffer
	cleanup, err := applyInsecureTxExemption(&opts, nil, &w)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if opts.FHIRSettings == "" {
		t.Fatal("want a settings file to be written")
	}
	data, err := os.ReadFile(opts.FHIRSettings) //nolint:gosec // path we just produced
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Servers []struct {
			URL                 string `json:"url"`
			AllowHTTP           bool   `json:"allowHttp"`
			AllowPrivateNetwork bool   `json:"allowPrivateNetwork"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("settings file is not valid JSON: %v\n%s", err, data)
	}
	if len(got.Servers) != 1 {
		t.Fatalf("want exactly one exempted server, got %d: %s", len(got.Servers), data)
	}
	s := got.Servers[0]
	if s.URL != "http://127.0.0.1:8080/fhir" || !s.AllowHTTP || !s.AllowPrivateNetwork {
		t.Errorf("exemption is wrong: %+v", s)
	}
	// A run that weakens a control, however narrowly, must say so.
	if !strings.Contains(w.String(), "SSRF") {
		t.Errorf("the exemption must be announced, got %q", w.String())
	}
}

func TestApplyInsecureTxExemption_NoOpCases(t *testing.T) {
	for name, opts := range map[string]validator.Options{
		"no opt-in":       {TerminologyServer: "http://127.0.0.1:8080/fhir"},
		"https server":    {TerminologyServer: "https://tx.fhir.org/r4", AllowInsecureTx: true},
		"no terminology":  {TerminologyServer: "http://127.0.0.1:8080/fhir", AllowInsecureTx: true, NoTerminologyServer: true},
		"no server named": {AllowInsecureTx: true},
		"settings already set": {
			TerminologyServer: "http://127.0.0.1:8080/fhir",
			AllowInsecureTx:   true,
			FHIRSettings:      "/already/written.json",
		},
	} {
		t.Run(name, func(t *testing.T) {
			before := opts.FHIRSettings
			var w bytes.Buffer
			cleanup, err := applyInsecureTxExemption(&opts, nil, &w)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			if opts.FHIRSettings != before {
				t.Errorf("must not touch the settings file here, got %q", opts.FHIRSettings)
			}
			if w.Len() != 0 {
				t.Errorf("must stay silent, said %q", w.String())
			}
		})
	}
}

// Replay writes its own settings file and the validator reads exactly one, so
// the exemption must not overwrite it.
func TestApplyInsecureTxExemption_LeavesReplaySettingsAlone(t *testing.T) {
	opts := validator.Options{
		TerminologyServer: "http://127.0.0.1:34567",
		AllowInsecureTx:   true,
		FHIRSettings:      "/tmp/replay-settings.json",
	}
	var w bytes.Buffer
	cleanup, err := applyInsecureTxExemption(&opts, nil, &w)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if opts.FHIRSettings != "/tmp/replay-settings.json" {
		t.Errorf("replay's settings file was replaced: %q", opts.FHIRSettings)
	}
}

// A -fhir-settings passed through by hand would silently lose to ours.
func TestApplyInsecureTxExemption_RejectsAPassthroughSettingsArg(t *testing.T) {
	opts := validator.Options{
		TerminologyServer: "http://127.0.0.1:8080/fhir",
		AllowInsecureTx:   true,
	}
	var w bytes.Buffer
	_, err := applyInsecureTxExemption(&opts, []string{"-fhir-settings", "mine.json"}, &w)
	if err == nil {
		t.Error("a hand-passed -fhir-settings must be refused rather than silently overridden")
	}
}
