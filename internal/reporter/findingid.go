// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package reporter

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// sarifFingerprintKey names fhirlint's hash in SARIF partialFingerprints. The
// version suffix changes whenever findingHash does, so code scanning treats a
// new scheme as new rather than silently mismatching (#543).
const sarifFingerprintKey = "fhirlintFinding/v1"

// findingHashes gives each issue of a report a stable identity: the same parts
// the baseline keys on — file relative to the working directory, message ID,
// location without line and column — plus the message text only when there is
// no message ID to go by, and an occurrence number that tells identical
// findings in one file apart. A reformatted file or a validator release that
// rewords a message keeps the identity, so GitLab and code scanning don't show
// every finding as resolved and new (#543). Issues are numbered in report
// order, which is the validator's.
type findingHashes struct {
	seen map[string]int
}

func newFindingHashes() *findingHashes { return &findingHashes{seen: map[string]int{}} }

func (h *findingHashes) next(filename string, iss validator.Issue) string {
	text := ""
	if iss.MessageID == "" {
		text = iss.Message
	}
	key := identityPath(filename) + "\x00" + iss.MessageID + "\x00" + stripLineCol(iss.Location) + "\x00" + text
	n := h.seen[key]
	h.seen[key] = n + 1
	sum := sha256.Sum256([]byte(key + "\x00" + strconv.Itoa(n)))
	return hex.EncodeToString(sum[:])
}

// identityPath is the file as the baseline names it: slash-separated, and
// relative to the working directory when it was absolute, so the identity
// doesn't depend on where the repository is checked out.
func identityPath(file string) string {
	if filepath.IsAbs(file) {
		if cwd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(cwd, file); err == nil {
				file = rel
			}
		}
	}
	return filepath.ToSlash(file)
}

// stripLineCol drops the " (line X, col Y)" suffix, exactly as the baseline
// does (internal/baseline normalizeLocation).
func stripLineCol(loc string) string {
	if i := strings.Index(loc, " (line "); i >= 0 {
		return loc[:i]
	}
	return loc
}
