package cmd

import (
	"fmt"
	"strings"

	"github.com/fhirlint/fhirlint/internal/explain"
	"github.com/spf13/cobra"
)

var explainCmd = &cobra.Command{
	Use:   "explain <messageId>",
	Short: "Explain a validation message ID and how to fix it",
	Long: `Print a description of a FHIR validation message ID and common remediation
advice, fully offline.

fhirlint ships explanations for common FHIR core invariants (dom-*, ele-1,
ext-1, bdl-*, obs-*). Run "fhirlint explain --list" to see all known IDs.`,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runExplain,
	SilenceUsage: true,
	ValidArgsFunction: func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var out []string
		for _, id := range explain.IDs() {
			if strings.HasPrefix(id, strings.ToLower(toComplete)) {
				out = append(out, id)
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	},
}

var flagExplainList bool

func init() {
	explainCmd.Flags().BoolVar(&flagExplainList, "list", false, "List all message IDs with a built-in explanation")
}

func runExplain(_ *cobra.Command, args []string) error {
	if flagExplainList {
		for _, id := range explain.IDs() {
			r, _ := explain.Lookup(id)
			fmt.Printf("%-8s %s\n", r.ID, r.Title)
		}
		return nil
	}

	if len(args) == 0 {
		return fmt.Errorf("provide a message ID (e.g. fhirlint explain dom-6) or use --list")
	}

	id := args[0]
	rule, ok := explain.Lookup(id)
	if !ok {
		return fmt.Errorf("no built-in explanation for %q — it is not a known FHIR core invariant; "+
			"look it up in the HL7 FHIR specification at https://hl7.org/fhir/", id)
	}

	fmt.Print(explain.Format(rule))
	return nil
}
