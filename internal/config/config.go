package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	DataDir    string          `mapstructure:"data_dir"`
	LLM        LLMConfig       `mapstructure:"llm"`
	Embeddings EmbeddingsConfig `mapstructure:"embeddings"`
	Sources    SourcesConfig   `mapstructure:"sources"`
}

type LLMConfig struct {
	Provider      string            `mapstructure:"provider"`
	Model         string            `mapstructure:"model"`
	BaseURL       string            `mapstructure:"base_url"`
	APIKey        string            `mapstructure:"api_key"`
	Headers       map[string]string `mapstructure:"headers"`
	SummaryPrompt string            `mapstructure:"summary_prompt"`
}

type EmbeddingsConfig struct {
	Provider string `mapstructure:"provider"`
	Model    string `mapstructure:"model"`
	APIKey   string `mapstructure:"api_key"`
}

type SourcesConfig struct {
	X        bool `mapstructure:"x"`
	Raindrop bool `mapstructure:"raindrop"`
	GitHub   bool `mapstructure:"github"`
}

func Load() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	defaultDataDir := filepath.Join(homeDir, ".xhub")

	viper.SetDefault("data_dir", defaultDataDir)
	viper.SetDefault("llm.provider", "anthropic")
	viper.SetDefault("llm.model", "claude-haiku-4-5-20251001")
	viper.SetDefault("embeddings.provider", "openai")
	viper.SetDefault("embeddings.model", "text-embedding-3-small")
	viper.SetDefault("sources.x", true)
	viper.SetDefault("sources.raindrop", true)
	viper.SetDefault("sources.github", true)

	// Environment variable overrides
	viper.SetEnvPrefix("XHUB")
	viper.AutomaticEnv()
	viper.BindEnv("data_dir", "XHUB_DATA_DIR")
	viper.BindEnv("llm.provider", "XHUB_LLM_PROVIDER")
	viper.BindEnv("llm.model", "XHUB_LLM_MODEL")
	viper.BindEnv("llm.base_url", "XHUB_LLM_BASE_URL")

	// Config file
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(defaultDataDir)

	// Read config file if exists (ignore error if not found)
	_ = viper.ReadInConfig()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Ensure data directory exists
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) DBPath() string {
	return filepath.Join(c.DataDir, "xhub.db")
}

func (c *Config) CacheDir() string {
	return filepath.Join(c.DataDir, "cache")
}
