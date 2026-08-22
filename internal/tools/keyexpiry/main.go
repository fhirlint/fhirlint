// Command keyexpiry prints how long the pinned HL7 signing key remains valid,
// as "<days>\t<YYYY-MM-DD>". CI uses it to open a rotation issue before the pin
// lapses, since an expired pin degrades JAR verification quietly (#358).
//
// It prints nothing and exits 0 when the key carries no expiry.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func main() {
	expiry, ok := validator.PinnedSigningKeyExpiry()
	if !ok {
		return
	}
	days := int(time.Until(expiry).Hours() / 24)
	fmt.Printf("%d\t%s\n", days, expiry.Format("2006-01-02"))
	if days < 0 {
		fmt.Fprintf(os.Stderr, "the pinned HL7 signing key expired on %s\n", expiry.Format("2006-01-02"))
		os.Exit(1)
	}
}
