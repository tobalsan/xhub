package indexer

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/liushuangls/go-anthropic/v2"
	"github.com/sashabaranov/go-openai"
	"github.com/user/xhub/internal/config"
)

var debugMode bool

// SetDebugMode enables debug logging for summarization
func SetDebugMode(enabled bool) {
	debugMode = enabled
}

// SummaryResult contains the LLM-generated summary and keywords
type SummaryResult struct {
	Summary  string
	Keywords string
	RawResponse string // For debugging purposes
}

// Summarizer generates summaries using LLM
type Summarizer struct {
	cfg *config.Config
}

func NewSummarizer(cfg *config.Config) *Summarizer {
	return &Summarizer{cfg: cfg}
}

const summaryPrompt = `Analyze this content and provide:
1. A concise 1-2 sentence summary of what this is about
2. 3-5 relevant keywords separated by commas

Format your response exactly as:
SUMMARY: <your summary>
KEYWORDS: <keyword1>, <keyword2>, <keyword3>

Content:
%s`

func (s *Summarizer) Summarize(content string) (*SummaryResult, error) {
	// Truncate content for LLM
	const maxContentLen = 10000
	if len(content) > maxContentLen {
		content = content[:maxContentLen]
	}

	prompt := fmt.Sprintf(summaryPrompt, content)

	var response string
	var err error

	switch s.cfg.LLM.Provider {
	case "anthropic":
		response, err = s.summarizeWithAnthropic(prompt)
	case "openai", "openrouter", "cerebras", "zai":
		response, err = s.summarizeWithOpenAI(prompt)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", s.cfg.LLM.Provider)
	}

	if err != nil {
		return nil, err
	}

	result := parseResponse(response)
	result.RawResponse = response
	return result, nil
}

func (s *Summarizer) summarizeWithAnthropic(prompt string) (string, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		apiKey = s.cfg.LLM.APIKey
	}
	if apiKey == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY not set (set in config.yaml or environment)")
	}

	client := anthropic.NewClient(apiKey)

	if debugMode {
		log.Printf("[DEBUG] Sending request to Anthropic with model %s", s.cfg.LLM.Model)
		log.Printf("[DEBUG] Prompt length: %d chars", len(prompt))
	}

	resp, err := client.CreateMessages(context.Background(), anthropic.MessagesRequest{
		Model:     anthropic.Model(s.cfg.LLM.Model),
		MaxTokens: 2000,
		Messages: []anthropic.Message{
			{
				Role:    anthropic.RoleUser,
				Content: []anthropic.MessageContent{{Type: "text", Text: &prompt}},
			},
		},
	})

	if err != nil {
		if debugMode {
			log.Printf("[DEBUG] Anthropic API Error: %v", err)
		}
		return "", err
	}

	if debugMode {
		log.Printf("[DEBUG] Anthropic response received: %d content blocks", len(resp.Content))
		if len(resp.Content) > 0 {
			log.Printf("[DEBUG] Anthropic response text: %q", resp.Content[0].GetText())
		}
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("empty response from Anthropic")
	}

	return resp.Content[0].GetText(), nil
}

func (s *Summarizer) summarizeWithOpenAI(prompt string) (string, error) {
	var apiKey string
	var baseURL string

	switch s.cfg.LLM.Provider {
	case "openrouter":
		apiKey = os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			apiKey = s.cfg.LLM.APIKey
		}
		baseURL = s.cfg.LLM.BaseURL
		if baseURL == "" {
			baseURL = "https://openrouter.ai/api/v1"
		}
	case "cerebras":
		apiKey = os.Getenv("CEREBRAS_API_KEY")
		if apiKey == "" {
			apiKey = s.cfg.LLM.APIKey
		}
		baseURL = s.cfg.LLM.BaseURL
	case "zai":
		apiKey = os.Getenv("ZAI_API_KEY")
		if apiKey == "" {
			apiKey = s.cfg.LLM.APIKey
		}
		baseURL = s.cfg.LLM.BaseURL
	default: // openai
		apiKey = os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			apiKey = s.cfg.LLM.APIKey
		}
	}

	if apiKey == "" {
		return "", fmt.Errorf("API key not set for provider %s (set in config.yaml or environment)", s.cfg.LLM.Provider)
	}

	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}

	client := openai.NewClientWithConfig(config)

	if debugMode {
		log.Printf("[DEBUG] Sending request to %s with model %s", baseURL, s.cfg.LLM.Model)
		log.Printf("[DEBUG] Prompt length: %d chars", len(prompt))
	}

	resp, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model:     s.cfg.LLM.Model,
		MaxTokens: 2000,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
	})

	if err != nil {
		if debugMode {
			log.Printf("[DEBUG] API Error: %v", err)
		}
		return "", err
	}

	if debugMode {
		log.Printf("[DEBUG] Response received: %d choices", len(resp.Choices))
		if len(resp.Choices) > 0 {
			log.Printf("[DEBUG] Response content: %q", resp.Choices[0].Message.Content)
			log.Printf("[DEBUG] Full message: %+v", resp.Choices[0].Message)
		}
		log.Printf("[DEBUG] Full response object: %+v", resp)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response from OpenAI")
	}

	return resp.Choices[0].Message.Content, nil
}

func parseResponse(response string) *SummaryResult {
	result := &SummaryResult{}

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SUMMARY:") {
			result.Summary = strings.TrimSpace(strings.TrimPrefix(line, "SUMMARY:"))
		} else if strings.HasPrefix(line, "KEYWORDS:") {
			result.Keywords = strings.TrimSpace(strings.TrimPrefix(line, "KEYWORDS:"))
		}
	}

	return result
}
