package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/user/xhub/internal/config"
	"github.com/user/xhub/internal/indexer"
)

var verboseFlag bool

var fetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch and index bookmarks from all sources",
	Long:  "Refresh the index by fetching bookmarks from X, Raindrop, and GitHub.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		return indexer.Fetch(cfg, true, verboseFlag) // force refresh
	},
}

func init() {
	fetchCmd.Flags().BoolVarP(&verboseFlag, "verbose", "v", false, "Show detailed processing steps")
	rootCmd.AddCommand(fetchCmd)
}
