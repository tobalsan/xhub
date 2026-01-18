package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/user/xhub/internal/config"
	"github.com/user/xhub/internal/db"
)

var (
	jsonOutput      bool
	plaintextOutput bool
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search bookmarks",
	Long:  "Search indexed bookmarks using hybrid semantic + keyword search.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		store, err := db.NewStore(cfg.DataDir)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer store.Close()

		results, err := store.Search(query, 20)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		if jsonOutput {
			return outputJSON(results)
		}
		if plaintextOutput {
			return outputPlaintext(results)
		}
		return outputDefault(results)
	},
}

func outputJSON(results []db.Bookmark) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func outputPlaintext(results []db.Bookmark) error {
	for _, r := range results {
		fmt.Printf("%s\t%s\t%s\n", r.Source, r.Title, r.URL)
	}
	return nil
}

func outputDefault(results []db.Bookmark) error {
	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}
	for i, r := range results {
		icon := sourceIcon(r.Source)
		fmt.Printf("%d. %s %s\n   %s\n", i+1, icon, r.Title, r.URL)
		if r.Summary != "" {
			fmt.Printf("   %s\n", truncate(r.Summary, 100))
		}
		fmt.Println()
	}
	return nil
}

func sourceIcon(source string) string {
	switch source {
	case "x":
		return "[X]"
	case "raindrop":
		return "[R]"
	case "github":
		return "[G]"
	case "manual":
		return "[M]"
	default:
		return "[?]"
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func init() {
	searchCmd.Flags().BoolVarP(&jsonOutput, "json", "j", false, "Output as JSON")
	searchCmd.Flags().BoolVarP(&plaintextOutput, "plaintext", "p", false, "Output as plaintext")
	rootCmd.AddCommand(searchCmd)
}
