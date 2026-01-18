package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/user/xhub/internal/config"
	"github.com/user/xhub/internal/tui"
)

var rootCmd = &cobra.Command{
	Use:   "xhub",
	Short: "Unified bookmarks search TUI",
	Long:  "A TUI app to index X bookmarks, Raindrop bookmarks, and GitHub starred repos with semantic search.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		return tui.Run(cfg)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().String("data-dir", "", "Data directory (default: ~/.xhub)")
}
