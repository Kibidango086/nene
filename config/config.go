package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type ProviderConfig struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	APIKey    string `json:"api_key"`
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	Timeout   int    `json:"timeout"`
	MaxTokens int    `json:"max_tokens"`
}

type Config struct {
	Telegram struct {
		Token      string   `json:"token"`
		Proxy      string   `json:"proxy"`
		AllowFrom  []string `json:"allow_from"`
		StreamMode bool     `json:"stream_mode"`
	} `json:"telegram"`
	Provider     ProviderConfig   `json:"provider"`
	Providers    []ProviderConfig `json:"providers"`
	SystemPrompt string           `json:"system_prompt"`
}

func ConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".nene")
}

func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

func DataDir() string {
	return ConfigDir()
}

func Init() error {
	dir := ConfigDir()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create config directory: %w", err)
		}
	}

	cfgPath := ConfigPath()
	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Printf("Config already exists at %s\n", cfgPath)
		return nil
	}

	cfg := &Config{
		Provider: ProviderConfig{
			ID:    "default",
			Type:  "openai",
			Model: "gpt-4o",
		},
	}
	cfg.Telegram.StreamMode = true

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Printf("Created config at %s\n", cfgPath)
	fmt.Println("Please edit the config file and add your telegram token and API key.")
	return nil
}

func Load() (*Config, error) {
	cfg := &Config{}

	if err := loadFromFile(cfg); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load config file: %w", err)
	}

	overrideWithEnv(cfg)

	if cfg.Telegram.Token == "" {
		return nil, fmt.Errorf("telegram token is required (set TELEGRAM_BOT_TOKEN env or telegram.token in %s)", ConfigPath())
	}

	if cfg.Provider.Type == "" {
		cfg.Provider.Type = "openai"
	}
	if cfg.Provider.Model == "" {
		cfg.Provider.Model = "gpt-4o"
	}

	switch cfg.Provider.Type {
	case "openai", "openai-compatible":
		if cfg.Provider.BaseURL == "" {
			cfg.Provider.BaseURL = "https://api.openai.com/v1"
		}
	case "anthropic":
		if cfg.Provider.BaseURL == "" {
			cfg.Provider.BaseURL = "https://api.anthropic.com/v1"
		}
	case "azure":
		if cfg.Provider.BaseURL == "" {
			cfg.Provider.BaseURL = "https://YOUR_RESOURCE.openai.azure.com"
		}
	}

	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = DefaultSystemPrompt
	}

	return cfg, nil
}

func loadFromFile(cfg *Config) error {
	cfgPath := ConfigPath()

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parse config file: %w", err)
	}

	return nil
}

func overrideWithEnv(cfg *Config) {
	if v := os.Getenv("TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.Telegram.Token = v
	}
	if v := os.Getenv("TELEGRAM_PROXY"); v != "" {
		cfg.Telegram.Proxy = v
	}
	if v := os.Getenv("NENE_PROVIDER_TYPE"); v != "" {
		cfg.Provider.Type = v
	}
	if v := os.Getenv("NENE_PROVIDER_API_KEY"); v != "" {
		cfg.Provider.APIKey = v
	}
	if v := os.Getenv("NENE_PROVIDER_BASE_URL"); v != "" {
		cfg.Provider.BaseURL = v
	}
	if v := os.Getenv("NENE_PROVIDER_MODEL"); v != "" {
		cfg.Provider.Model = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" && cfg.Provider.APIKey == "" {
		cfg.Provider.APIKey = v
		cfg.Provider.Type = "openai"
	}
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" && cfg.Provider.BaseURL == "" {
		cfg.Provider.BaseURL = v
	}
	if v := os.Getenv("OPENAI_MODEL"); v != "" && cfg.Provider.Model == "" {
		cfg.Provider.Model = v
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" && cfg.Provider.APIKey == "" {
		cfg.Provider.APIKey = v
		cfg.Provider.Type = "anthropic"
	}
	if v := os.Getenv("AZURE_OPENAI_API_KEY"); v != "" && cfg.Provider.APIKey == "" {
		cfg.Provider.APIKey = v
		cfg.Provider.Type = "azure"
	}
	if v := os.Getenv("NENE_SYSTEM_PROMPT"); v != "" {
		cfg.SystemPrompt = v
	}
}

func hostOS() string {
	switch runtime.GOOS {
	case "linux":
		return "Linux"
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	default:
		return runtime.GOOS
	}
}

var DefaultSystemPrompt = fmt.Sprintf(`You are Nene, a helpful AI assistant accessible via Telegram.

You can help users with various tasks including:
- Answering questions
- Writing and editing code
- Running shell commands
- Reading and writing files
- General problem-solving

When using tools:
- Be clear about what you're doing
- Show the results to the user
- Ask for confirmation if a tool action might be destructive

Current platform: %s
`, hostOS())
