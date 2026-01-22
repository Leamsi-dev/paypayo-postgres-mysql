package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Database DatabaseConfig `yaml:"database"`
	Listener ListenerConfig `yaml:"listener"`
	Webhook  WebhookConfig  `yaml:"webhook"`
	Logging  LoggingConfig  `yaml:"logging"`
	Worker   WorkerConfig   `yaml:"worker"`
}

type DatabaseConfig struct {
	Type     string `yaml:"type"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	Table    string `yaml:"table"`
	SSLMode  string `yaml:"sslmode"`
}

type ListenerConfig struct {
	Modes        string `yaml:"modes"`
	PollInterval int    `yaml:"poll_interval"`
}

type WebhookConfig struct {
	URL        string `yaml:"url"`
	Timeout    int    `yaml:"timeout"`
	RetryCount int    `yaml:"retry_count"`
	RetryDelay int    `yaml:"retry_delay"`
}

type LoggingConfig struct {
	File  string `yaml:"file"`
	Level string `yaml:"level"`
}

type WorkerConfig struct {
	PoolSize int `yaml:"pool_size"`
}

func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("erreur lecture fichier config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("erreur parsing config: %w", err)
	}

	return &cfg, nil
}

func (c *ListenerConfig) IsInsertEnabled() bool {
	return strings.Contains(strings.ToLower(c.Modes), "insert")
}

func (c *ListenerConfig) IsUpdateEnabled() bool {
	return strings.Contains(strings.ToLower(c.Modes), "update")
}

func (c *ListenerConfig) IsDeleteEnabled() bool {
	return strings.Contains(strings.ToLower(c.Modes), "delete")
}
