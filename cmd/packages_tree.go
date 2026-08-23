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
