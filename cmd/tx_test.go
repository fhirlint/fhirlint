package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/fhirlint/fhirlint/internal/txreplay"
)

func TestTxRecordingDir_FlagBeatsConfigBeatsDefault(t *testing.T) {
	resetViper(t)

	if got := txRecordingDir(""); got != txreplay.DefaultDir {
		t.Errorf("with nothing set, expected %q, got %q", txreplay.DefaultDir, got)
	}

	viper.Set("tx-dir", "from-config")
	if got := txRecordingDir(""); got != "from-config" {
		t.Errorf("config should win over the default, got %q", got)
	}
	if got := txRecordingDir("from-flag"); got != "from-flag" {
		t.Errorf("flag should win over config, got %q", got)
	}
}

func TestTxRecordingExcludes_KeepsRecordingOutOfTheInputSet(t *testing.T) {
	resetViper(t)

	got := txRecordingExcludes("")
	if len(got) != 1 || got[0] != txreplay.DefaultDir+"/**" {
		t.Fatalf("expected the default recording dir to be excluded, got %v", got)
	}
	// The pattern has to actually match what lands under the recording dir,
	// otherwise recordings get validated as if they were FHIR resources.
	if !matchesExclude(txreplay.DefaultDir+"/abc123.json", got[0]) {
		t.Errorf("pattern %q does not match a recording file", got[0])
	}
	if matchesExclude("fhir/patient.json", got[0]) {
		t.Errorf("pattern %q must not match ordinary resources", got[0])
	}
}

func TestTxRecordingExcludes_HonoursTrailingSlash(t *testing.T) {
	resetViper(t)

	got := txRecordingExcludes("recordings/")
	if len(got) != 1 || got[0] != "recordings/**" {
		t.Fatalf("expected recordings/**, got %v", got)
	}
}

func TestTxMissError_NilWhenNothingMissed(t *testing.T) {
	if err := txMissError(nil, ".fhirlint-tx", "./fhir"); err != nil {
		t.Errorf("no misses should not be an error, got %v", err)
	}
}

func TestTxMissError_NamesWhatWasMissingAndHowToFixIt(t *testing.T) {
	misses := []txreplay.Miss{
		{Method: "POST", Path: "/ValueSet/$validate-code", Detail: "system http://loinc.org, code 8302-2"},
	}

	err := txMissError(misses, ".fhirlint-tx", "./fhir")
	if err == nil {
		t.Fatal("a miss must fail the run")
	}
	msg := err.Error()
	for _, want := range []string{"8302-2", "http://loinc.org", ".fhirlint-tx", "fhirlint tx warm ./fhir"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should mention %q, got:\n%s", want, msg)
		}
	}
}

func TestTxMissError_TruncatesLongLists(t *testing.T) {
	misses := make([]txreplay.Miss, maxReportedMisses+5)
	for i := range misses {
		misses[i] = txreplay.Miss{Method: "POST", Path: "/ValueSet/$validate-code"}
	}

	msg := txMissError(misses, ".fhirlint-tx", ".").Error()
	if !strings.Contains(msg, "and 5 more") {
		t.Errorf("expected the list to be truncated, got:\n%s", msg)
	}
	if got := strings.Count(msg, "/ValueSet/$validate-code"); got != maxReportedMisses {
		t.Errorf("expected %d listed misses, got %d", maxReportedMisses, got)
	}
}

func TestConfigFile_TxOfflineFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "tx-offline: true\ntx-dir: recordings\n")

	viper.SetConfigFile(dir + "/fhirlint.yml")
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}

	if !viper.GetBool("tx-offline") {
		t.Error("tx-offline should be true")
	}
	if got := viper.GetString("tx-dir"); got != "recordings" {
		t.Errorf("tx-dir = %q, want %q", got, "recordings")
	}
}
