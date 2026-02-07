package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/user/xhub/internal/config"
	"github.com/user/xhub/internal/indexer"
)

var (
	reprocessVerbose bool
)

var reprocessCmd = &cobra.Command{
	Use:   "reprocess <id-or-url>",
	Short: "Reprocess a single bookmark",
	Long:  "Re-scrape, re-summarize, and re-embed one bookmark by ID or URL.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		b, err := indexer.ReprocessByIDOrURL(cfg, args[0], reprocessVerbose)
		if err != nil {
			return fmt.Errorf("reprocess failed: %w", err)
		}

		fmt.Printf("Reprocessed: %s\n", b.URL)
		fmt.Printf("Title: %s\n", b.Title)
		if b.Summary != "" {
			fmt.Printf("Summary: %s\n", b.Summary)
		}
		return nil
	},
}

func init() {
	reprocessCmd.Flags().BoolVarP(&reprocessVerbose, "verbose", "v", false, "Show warnings for embedding issues")
	rootCmd.AddCommand(reprocessCmd)
}
