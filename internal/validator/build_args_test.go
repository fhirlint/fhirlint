package validator

import (
	"testing"
)

func TestBuildArgs_RequiredFlags(t *testing.T) {
	args := buildArgs("/fake/validator.jar", "/tmp/patient.json", "/tmp/out.json", Options{
		FHIRVersion: "4.0.1",
	})

	mustContainPair(t, args, "-jar", "/fake/validator.jar")
	mustContainPair(t, args, "-version", "4.0.1")
	mustContainPair(t, args, "-output-style", "json")
	mustContainPair(t, args, "-output", "/tmp/out.json")
	mustContain(t, args, "/tmp/patient.json")
}

func TestBuildArgs_NoTerminologyServer(t *testing.T) {
	args := buildArgs("jar", "input", "out", Options{
		FHIRVersion:         "4.0.1",
		NoTerminologyServer: true,
	})

	mustContainPair(t, args, "-tx", "n/a")
}

func TestBuildArgs_CustomTerminologyServer(t *testing.T) {
	args := buildArgs("jar", "input", "out", Options{
		FHIRVersion:       "4.0.1",
		TerminologyServer: "https://my-tx.example.com",
	})

	mustContainPair(t, args, "-tx", "https://my-tx.example.com")
}

func TestBuildArgs_NoTerminologyServerTakesPrecedence(t *testing.T) {
	args := buildArgs("jar", "input", "out", Options{
		FHIRVersion:         "4.0.1",
		NoTerminologyServer: true,
		TerminologyServer:   "https://my-tx.example.com",
	})

	mustContainPair(t, args, "-tx", "n/a")
	mustNotContain(t, args, "https://my-tx.example.com")
}

func TestBuildArgs_DefaultHasNoTxFlag(t *testing.T) {
	args := buildArgs("jar", "input", "out", Options{FHIRVersion: "4.0.1"})

	mustNotContain(t, args, "-tx")
}

func TestBuildArgs_Profiles(t *testing.T) {
	args := buildArgs("jar", "input", "out", Options{
		FHIRVersion: "4.0.1",
		Profiles:    []string{"http://example.com/profile1", "http://example.com/profile2"},
	})

	mustContainPair(t, args, "-profile", "http://example.com/profile1")
	mustContainPair(t, args, "-profile", "http://example.com/profile2")
}

func TestBuildArgs_IGs(t *testing.T) {
	args := buildArgs("jar", "input", "out", Options{
		FHIRVersion: "4.0.1",
		IGs:         []string{"kbv.basis#1.5.0", "de.medizininformatikinitiative.kerndatensatz#2024.0.0"},
	})

	mustContainPair(t, args, "-ig", "kbv.basis#1.5.0")
	mustContainPair(t, args, "-ig", "de.medizininformatikinitiative.kerndatensatz#2024.0.0")
}

func TestBuildArgs_FHIRVersions(t *testing.T) {
	for _, version := range []string{"4.0.1", "4.3.0", "5.0.0"} {
		args := buildArgs("jar", "input", "out", Options{FHIRVersion: version})
		mustContainPair(t, args, "-version", version)
	}
}

func TestBuildArgs_EmptyProfilesAndIGs(t *testing.T) {
	args := buildArgs("jar", "input", "out", Options{
		FHIRVersion: "4.0.1",
		Profiles:    []string{},
		IGs:         []string{},
	})

	mustNotContain(t, args, "-profile")
	mustNotContain(t, args, "-ig")
}

// mustContainPair asserts that args contains flag immediately followed by value.
func mustContainPair(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return
		}
	}
	t.Errorf("expected args to contain %q %q, got: %v", flag, value, args)
}

// mustContain asserts that args contains the given value.
func mustContain(t *testing.T, args []string, value string) {
	t.Helper()
	for _, a := range args {
		if a == value {
			return
		}
	}
	t.Errorf("expected args to contain %q, got: %v", value, args)
}

// mustNotContain asserts that args does not contain the given value.
func mustNotContain(t *testing.T, args []string, value string) {
	t.Helper()
	for _, a := range args {
		if a == value {
			t.Errorf("expected args NOT to contain %q, got: %v", value, args)
			return
		}
	}
}
