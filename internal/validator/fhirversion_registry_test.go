//go:build registry

// This file checks the FHIR version table against the outside world: the
// package registry and tx.fhir.org. It is behind a build tag for the same
// reason as the alias check — `go test ./...` must not depend on someone
// else's server being up.
//
//	go test -tags registry -v ./internal/validator/
package validator_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/internal/igaudit"
	"github.com/fhirlint/fhirlint/internal/validator"
)

const (
	txBaseURL     = "https://tx.fhir.org"
	probeTimeout  = 20 * time.Second
	r6CorePackage = "hl7.fhir.r6.core" // what VersionUtilities.packageForVersion returns for R6
	r6TxPath      = "/r6"
)

// reachable reports whether a GET returns 200. A transport error is reported
// separately from a 404: an outage says nothing about whether a resource
// exists.
func reachable(t *testing.T, url string) (ok bool, err error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK, nil
}

// TestFHIRVersionsResolve enforces the contract the table states about itself:
// TxPath and CorePackage are "verified rather than guessed". This is what
// verifies them.
func TestFHIRVersionsResolve(t *testing.T) {
	client := igaudit.NewClient()

	for _, v := range validator.FHIRVersions {
		t.Run(v.ID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
			defer cancel()

			report := igaudit.Audit(ctx, client, []string{v.CorePackage + "#" + v.ID})
			if len(report.Packages) != 1 {
				t.Fatalf("audit returned %d reports for one package", len(report.Packages))
			}
			p := report.Packages[0]
			switch {
			case p.Error != "":
				t.Logf("could not check %s: %s", p.ID, p.Error)
			case p.NotFound:
				t.Errorf("core package %s does not exist — the table names a package nothing can fetch", v.CorePackage)
			case p.VersionMissing:
				t.Errorf("core package %s has no version %s", v.CorePackage, v.ID)
			}

			ok, err := reachable(t, txBaseURL+v.TxPath+"/metadata")
			switch {
			case err != nil:
				t.Logf("could not reach %s%s: %v", txBaseURL, v.TxPath, err)
			case !ok:
				t.Errorf("tx.fhir.org serves no %s endpoint, but the table sends %s there", v.TxPath, v.Name)
			}
		})
	}
}

// TestR6NotYetAvailable is a tripwire, not a check of our own code.
//
// R6 is deliberately absent from the table: as of 2026-08-21 there is no
// hl7.fhir.r6.core package on the registry and tx.fhir.org serves no /r6
// endpoint, so both fields a row needs would have to be guessed — which the
// table's own comment forbids. The JAR meanwhile maps any R6 version to
// 6.0.0-ballot3 while the spec build is at ballot4.
//
// This fails when that stops being true, which is the moment to add the row.
// Delete this test with the row (#306).
func TestR6NotYetAvailable(t *testing.T) {
	for _, v := range validator.FHIRVersions {
		if v.Name == "R6" {
			t.Skip("R6 is in the table already — this tripwire has done its job and can go")
		}
	}

	client := igaudit.NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	report := igaudit.Audit(ctx, client, []string{r6CorePackage + "#6.0.0"})
	pkgExists := len(report.Packages) == 1 && !report.Packages[0].NotFound && report.Packages[0].Error == ""

	txExists, err := reachable(t, txBaseURL+r6TxPath+"/metadata")
	if err != nil {
		t.Logf("could not reach %s%s: %v", txBaseURL, r6TxPath, err)
	}

	if pkgExists && txExists {
		t.Errorf("R6 has what the table needs: %s is on the registry and tx.fhir.org serves %s. "+
			"Add the row to validator.FHIRVersions, regenerate the schema, update the README table, and delete this test (#306).",
			r6CorePackage, r6TxPath)
	}
	t.Logf("R6 prerequisites: core package present=%v, %s endpoint present=%v", pkgExists, r6TxPath, txExists)
}
