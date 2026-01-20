package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/user/xhub/internal/config"
	"github.com/user/xhub/internal/indexer"
)

var (
	verboseFlag   bool
	forceFlag     bool
	reprocessFlag bool
	sourceFlag    []string
)

var fetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch and index bookmarks from all sources",
	Long:  "Refresh the index by fetching bookmarks from X, Raindrop, and GitHub.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Normalize source names
		var sources []string
		for _, s := range sourceFlag {
			sources = append(sources, strings.ToLower(strings.TrimSpace(s)))
		}

		return indexer.Fetch(cfg, indexer.FetchOptions{
			Force:     forceFlag,
			Reprocess: reprocessFlag,
			Verbose:   verboseFlag,
			Sources:   sources,
		})
	},
}

func init() {
	fetchCmd.Flags().BoolVarP(&verboseFlag, "verbose", "v", false, "Show detailed processing steps")
	fetchCmd.Flags().BoolVarP(&forceFlag, "force", "f", false, "Full reimport of all bookmarks from sources")
	fetchCmd.Flags().BoolVarP(&reprocessFlag, "reprocess", "r", false, "Re-scrape, re-summarize, and re-embed existing items (use with --force)")
	fetchCmd.Flags().StringSliceVarP(&sourceFlag, "source", "s", nil, "Filter to specific source(s): github, x, raindrop")
	rootCmd.AddCommand(fetchCmd)
}
