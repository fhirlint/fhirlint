package validator

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
)

const minJavaVersion = 11

func CheckJava() error {
	out, err := exec.Command("java", "-version").CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"java not found. fhirlint requires Java %d+.\n"+
				"install it from https://adoptium.net or via your package manager:\n"+
				"  Ubuntu/Debian: sudo apt install default-jre\n"+
				"  macOS:         brew install openjdk\n"+
				"  Windows:       winget install Microsoft.OpenJDK.21",
			minJavaVersion,
		)
	}

	major, err := parseJavaMajorVersion(string(out))
	if err != nil {
		return fmt.Errorf("could not determine Java version: %w", err)
	}

	if major < minJavaVersion {
		return fmt.Errorf(
			"Java %d detected, but fhirlint requires Java %d+.\n"+
				"install a newer version from https://adoptium.net or via your package manager:\n"+
				"  Ubuntu/Debian: sudo apt install default-jre\n"+
				"  macOS:         brew install openjdk\n"+
				"  Windows:       winget install Microsoft.OpenJDK.21",
			major, minJavaVersion,
		)
	}

	return nil
}

// parseJavaMajorVersion extracts the major version number from `java -version` output.
// Java 8 and below reports "1.X.Y", Java 9+ reports "X.Y.Z".
var javaVersionRe = regexp.MustCompile(`version "(\d+)(?:\.(\d+))?`)

func parseJavaMajorVersion(output string) (int, error) {
	m := javaVersionRe.FindStringSubmatch(output)
	if m == nil {
		return 0, fmt.Errorf("unexpected java -version output: %s", output)
	}

	first, _ := strconv.Atoi(m[1])
	if first == 1 && m[2] != "" {
		// Legacy format: 1.8.0 → major is the second component
		return strconv.Atoi(m[2])
	}
	return first, nil
}
