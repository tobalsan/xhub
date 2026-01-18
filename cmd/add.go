package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/user/xhub/internal/config"
	"github.com/user/xhub/internal/indexer"
)

var addCmd = &cobra.Command{
	Use:   "add <url>",
	Short: "Add a manual bookmark",
	Long:  "Add a URL as a manual bookmark to the index.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		url := args[0]

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if err := indexer.AddManualURL(cfg, url); err != nil {
			return fmt.Errorf("failed to add URL: %w", err)
		}

		fmt.Printf("Added: %s\n", url)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
}
