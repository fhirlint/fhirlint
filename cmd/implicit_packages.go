package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/fhirlint/fhirlint/internal/iglock"
	"github.com/fhirlint/fhirlint/internal/termsafe"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// reportImplicitPackages says, once per run, which packages the validator
// loaded on its own (#520). It met a canonical in the input and loaded the
// newest version of the package that defines it, pre-releases included, so a
// newly published version can change the findings without any change here.
//
// Informational, not an issue in the results: the fix is in the run's
// configuration (--ig id#version), not in the file, and fhirlint.lock is what
// turns a change into a failure.
func reportImplicitPackages(w io.Writer, implicit []validator.PackageRef, requested []string) {
	if len(implicit) == 0 {
		return
	}
	names := make([]string, len(implicit))
	var pre []string
	for i, p := range implicit {
		names[i] = termsafe.Line(p.String())
		if p.PreRelease {
			pre = append(pre, names[i])
		}
	}
	_, _ = fmt.Fprintf(w, "note: the validator loaded %d package(s) that no --ig, fhirlint.yml or %s named, and the findings depend on them: %s\n",
		len(implicit), iglock.LockFileName, strings.Join(names, ", "))
	if len(pre) > 0 {
		_, _ = fmt.Fprintf(w, "warn: %d of them are pre-releases: %s — pin the version you mean with --ig <id>#<version>\n",
			len(pre), strings.Join(pre, ", "))
	}

	requestedVersion := map[string][]string{}
	for _, ig := range requested {
		if id, version := iglock.ParseIGID(ig); id != "" {
			requestedVersion[id] = append(requestedVersion[id], termsafe.Line(id+"#"+version))
		}
	}
	for _, p := range implicit {
		if versions, ok := requestedVersion[p.ID]; ok {
			_, _ = fmt.Fprintf(w, "warn: the validator loaded %s although %s was requested — findings may come from either version\n",
				termsafe.Line(p.String()), strings.Join(versions, ", "))
		}
	}
}
