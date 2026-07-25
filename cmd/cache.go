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
