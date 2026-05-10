package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fhirlint/fhirlint/internal/resultcache"
	"github.com/spf13/cobra"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage the validation result cache",
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Remove all cached validation results",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, _ := cmd.Flags().GetString("cache-dir")
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("resolving home dir: %w", err)
			}
			dir = filepath.Join(home, ".fhirlint", "result-cache")
		}
		n, err := resultcache.Clear(dir)
		if err != nil {
			return fmt.Errorf("clearing cache: %w", err)
		}
		fmt.Printf("Removed %d cached result(s) from %s\n", n, dir)
		return nil
	},
}

func init() {
	cacheClearCmd.Flags().String("cache-dir", "",
		"Cache directory to clear (default: ~/.fhirlint/result-cache/)")
	cacheCmd.AddCommand(cacheClearCmd)
	rootCmd.AddCommand(cacheCmd)
}
