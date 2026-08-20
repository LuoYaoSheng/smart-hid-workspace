// Package config 加载 ControlHub 配置。
// Phase 1：YAML 文件（可选）+ 默认值；不做环境变量覆盖（Phase 5 接入）。
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config 是 ControlHub 运行配置。
type Config struct {
	HTTP     HTTPConfig     `yaml:"http"`
	MQTT     MQTTConfig     `yaml:"mqtt"`
	APIKey   string         `yaml:"api_key"`
	DataDir  string         `yaml:"data_dir"`
	LogLevel string         `yaml:"log_level"`
}

type HTTPConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}


type MQTTConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Default 返回内置默认配置（无 config.yaml 时使用）。
func Default() *Config {
	return &Config{
		HTTP:     HTTPConfig{Host: "127.0.0.1", Port: 17890},
		MQTT:     MQTTConfig{Host: "127.0.0.1", Port: 17891, Username: "controlhub", Password: "change-me-in-production"},
		APIKey:   "",
		DataDir:  "./data",
		LogLevel: "info",
	}
}

// Load 从 path 加载配置；path 为空或文件不存在则返回 Default。
// 加载后对关键字段做 sanitize（补默认值、建目录）。
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
		cfg.HTTP.Port = 17890
	}
	if cfg.MQTT.Port == 0 {
		cfg.MQTT.Port = 17891
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	// 确保 data_dir 存在（abs 化前）
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir data_dir %s: %w", cfg.DataDir, err)
	}
	if abs, err := filepath.Abs(cfg.DataDir); err == nil {
		cfg.DataDir = abs
	}

	return cfg, nil
}
