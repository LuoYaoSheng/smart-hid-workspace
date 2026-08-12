// Package config 加载 Smart HID Cloud 配置。
// 复用 ControlHub 的 YAML 加载模式（CH-P1）。
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config 是 Cloud 运行配置。
type Config struct {
	HTTP            HTTPConfig     `yaml:"http"`
	Database        DatabaseConfig `yaml:"database"`
	JWTSecret       string         `yaml:"jwt_secret"`
	LicenseKeyPath  string         `yaml:"license_key_path"`
	ControlHubToken string         `yaml:"controlhub_token"` // ControlHub 调 Cloud 时的共享密钥（V1 简化鉴权）
	DataDir         string         `yaml:"data_dir"`
	LogLevel        string         `yaml:"log_level"`
}

type HTTPConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// Default 返回内置默认配置。
func Default() *Config {
	return &Config{
		HTTP:            HTTPConfig{Host: "127.0.0.1", Port: 17880},
		Database:        DatabaseConfig{Path: "./data/cloud.db"},
		JWTSecret:       "change-me-in-production",
		LicenseKeyPath:  "./keys/private.key",
		ControlHubToken: "",
		DataDir:         "./data",
		LogLevel:        "info",
	}
}

// Load 从 path 加载配置；path 为空或文件不存在则返回 Default。
func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read config %s: %w", path, err)
			}
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parse config %s: %w", path, err)
			}
		}
	}

	// sanitize
	if cfg.HTTP.Port == 0 {
		cfg.HTTP.Port = 17880
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "./data/cloud.db"
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.LicenseKeyPath == "" {
		cfg.LicenseKeyPath = "./keys/private.key"
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir data_dir %s: %w", cfg.DataDir, err)
	}
	// 确保数据库目录存在
	dbDir := filepath.Dir(cfg.Database.Path)
	if dbDir != "" && dbDir != "." {
		_ = os.MkdirAll(dbDir, 0o755)
	}

	return cfg, nil
}
