package cmd

import (
	"fmt"
	"io"

	"github.com/fhirlint/fhirlint/internal/localig"
)

// warnSkippedIGFiles names every file among the local -ig entries that the
// validator will drop because of its name. The JAR only logs the rejection, so
// without this a profile in such a file is silently missing and instances
// claiming it pass (#444).
func warnSkippedIGFiles(w io.Writer, igs []string) {
	for _, path := range localig.SkippedByValidator(igs) {
		_, _ = fmt.Fprintf(w, "warning: the validator will not load %s — it skips local IG files whose name ends in "+
			"\"template.json\" or \"template.xml\" (hapifhir/org.hl7.fhir.core#2682). Rename the file to load it.\n", path)
	}
}
