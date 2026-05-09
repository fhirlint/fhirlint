package validator

import (
	"fmt"
	"os/exec"
	"strings"
)

func CheckJava() error {
	out, err := exec.Command("java", "-version").CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"Java not found. fhirlint requires Java 11+.\n" +
				"Install it from https://adoptium.net or via your package manager:\n" +
				"  Ubuntu/Debian: sudo apt install default-jre\n" +
				"  macOS:         brew install openjdk\n" +
				"  Windows:       winget install Microsoft.OpenJDK.21",
		)
	}
	if !strings.Contains(string(out), "version") {
		return fmt.Errorf("unexpected java -version output: %s", out)
	}
	return nil
}
