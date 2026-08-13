package validator

import (
	"strings"
	"testing"
)

func TestFHIRVersions_TableIsComplete(t *testing.T) {
	// A release added with a field left blank would fail somewhere far from
	// here: an empty TxPath silently points terminology recording at the bare
	// server, an empty CorePackage produces an unloadable temp IG.
	seen := map[string]bool{}
	for _, v := range FHIRVersions {
		if v.ID == "" || v.Name == "" || v.TxPath == "" || v.CorePackage == "" {
			t.Errorf("incomplete entry: %+v", v)
		}
		if seen[v.ID] {
			t.Errorf("duplicate version %q", v.ID)
		}
		seen[v.ID] = true
		if !strings.HasPrefix(v.TxPath, "/") {
			t.Errorf("%s: TxPath %q should be a path segment starting with /", v.ID, v.TxPath)
		}
		if !strings.HasPrefix(v.CorePackage, "hl7.fhir.") || !strings.HasSuffix(v.CorePackage, ".core") {
			t.Errorf("%s: CorePackage %q does not look like an hl7.fhir.rX.core package", v.ID, v.CorePackage)
		}
	}
}

func TestDefaultFHIRVersion_IsInTheTable(t *testing.T) {
	if _, ok := LookupFHIRVersion(DefaultFHIRVersion); !ok {
		t.Fatalf("DefaultFHIRVersion %q is not a supported release", DefaultFHIRVersion)
	}
	if err := validateFHIRVersion(DefaultFHIRVersion); err != nil {
		t.Errorf("the default must pass validation: %v", err)
	}
}

func TestFHIRVersionIDs_MatchesTableOrder(t *testing.T) {
	ids := FHIRVersionIDs()
	if len(ids) != len(FHIRVersions) {
		t.Fatalf("got %d ids for %d versions", len(ids), len(FHIRVersions))
	}
	for i, v := range FHIRVersions {
		if ids[i] != v.ID {
			t.Errorf("ids[%d] = %q, want %q", i, ids[i], v.ID)
		}
	}
}

func TestFHIRVersionList_ReadsAsFlagHelp(t *testing.T) {
	if got, want := FHIRVersionList(), "4.0.1, 4.3.0, 5.0.0"; got != want {
		t.Errorf("FHIRVersionList() = %q, want %q", got, want)
	}
}

func TestFHIRVersionName(t *testing.T) {
	cases := map[string]string{
		"4.0.1": "R4",
		"4.3.0": "R4B",
		"5.0.0": "R5",
		// Echoed back rather than blanked: a version fhirlint does not know is
		// still more useful printed than swallowed.
		"6.0.0": "6.0.0",
		"":      "",
	}
	for in, want := range cases {
		if got := FHIRVersionName(in); got != want {
			t.Errorf("FHIRVersionName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLookupFHIRVersion_Unknown(t *testing.T) {
	if v, ok := LookupFHIRVersion("6.0.0-ballot4"); ok {
		t.Errorf("a ballot version must not be accepted, got %+v", v)
	}
}

// Every release in the table has to be accepted by the gate, and the error for
// one that is not has to name them all — those two lists were separate copies
// before (#306).
func TestValidateFHIRVersion_AgreesWithTheTable(t *testing.T) {
	for _, v := range FHIRVersions {
		if err := validateFHIRVersion(v.ID); err != nil {
			t.Errorf("validateFHIRVersion(%q) = %v, want nil", v.ID, err)
		}
	}
	err := validateFHIRVersion("6.0.0")
	if err == nil {
		t.Fatal("expected an error for an unsupported version")
	}
	for _, v := range FHIRVersions {
		if !strings.Contains(err.Error(), v.ID) {
			t.Errorf("error should list %q, got: %v", v.ID, err)
		}
	}
}

func TestDefaultTerminologyEndpoint_ComesFromTheTable(t *testing.T) {
	for _, v := range FHIRVersions {
		want := DefaultTerminologyServer + v.TxPath
		if got := DefaultTerminologyEndpoint(v.ID); got != want {
			t.Errorf("DefaultTerminologyEndpoint(%q) = %q, want %q", v.ID, got, want)
		}
	}
}
