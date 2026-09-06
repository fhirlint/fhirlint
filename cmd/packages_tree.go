package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fhirlint/fhirlint/internal/fhirpkg"
)

func runPackagesTree(id string) error {
	name, version := fhirpkg.SplitID(id)
	if version == "" {
		return fmt.Errorf("give a package as name#version, for example kbv.basis#1.9.0 (got %q)", id)
	}
	root, err := fhirpkg.Tree(name, version, flagTreeDepth)
	if err != nil {
		return err
	}
	if flagPackagesJSON {
		return json.NewEncoder(os.Stdout).Encode(root)
	}
	printTree(root)
	return nil
}

func printTree(root *fhirpkg.Node) {
	fmt.Printf("%s#%s\n", root.Name, root.Version)
	for i, c := range root.Children {
		printTreeNode(c, "", i == len(root.Children)-1)
	}

	missing, inexact, truncated := fhirpkg.Counts(root)
	fmt.Println()
	if truncated {
		fmt.Printf("Stopped at --depth %d; deeper dependencies were not examined.\n", flagTreeDepth)
		return
	}
	if missing > 0 {
		fmt.Printf("%d dependency/dependencies missing from the cache — a run with --offline would fail.\n", missing)
	} else {
		fmt.Println("All dependencies are in the cache.")
	}
	if inexact > 0 {
		fmt.Printf("%d declared as a version range; the resolved version shown is what is installed now.\n", inexact)
	}
	if n := countAmbiguous(root); n > 0 {
		fmt.Printf("%d could not be ordered against the other installed versions; those are a guess.\n", n)
	}
}

func printTreeNode(n *fhirpkg.Node, prefix string, last bool) {
	branch, cont := "├── ", "│   "
	if last {
		branch, cont = "└── ", "    "
	}

	label := n.Name
	switch {
	case !n.Installed:
		label += "#" + n.Constraint + "   MISSING"
	case n.Ambiguous:
		// The candidates do not order against each other, so this is a stable
		// guess. Presenting it like a resolution would be the same silent
		// confidence #390 was about.
		label += "#" + n.Version + "   (declared " + n.Constraint + ", ambiguous)"
	case !n.Exact:
		label += "#" + n.Version + "   (declared " + n.Constraint + ")"
	default:
		label += "#" + n.Version
	}
	if n.Repeated || n.Truncated {
		label += "   …"
	}
	fmt.Println(prefix + branch + label)

	for i, c := range n.Children {
		printTreeNode(c, prefix+cont, i == len(n.Children)-1)
	}
}

// countAmbiguous counts ranges whose candidates had no order between them.
func countAmbiguous(n *fhirpkg.Node) int {
	total := 0
	if n.Ambiguous {
		total++
	}
	for _, c := range n.Children {
		total += countAmbiguous(c)
	}
	return total
}
