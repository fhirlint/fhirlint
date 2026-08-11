package cmd

import (
	"fmt"

	"github.com/fhirlint/fhirlint/internal/cache"
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
	// A cache that could not be cleared is a runtime failure, not a usage
	// mistake — printing the flag list on top of it buries the reason.
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, _ := cmd.Flags().GetString("cache-dir")
		if dir == "" {
			resolved, err := cache.ResultCacheDir()
			if err != nil {
				return fmt.Errorf("resolving cache dir: %w", err)
			}
			dir = resolved
		}
		n, err := resultcache.Clear(dir)
		if err != nil {
			// Say what did go, then fail: a partial clear leaves the cache in a
			// state the user needs to know about, and the count is part of that.
			if n > 0 {
				fmt.Printf("Removed %d cached result(s) from %s\n", n, dir)
			}
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
