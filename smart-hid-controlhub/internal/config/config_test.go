package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	c := Default()
	if c.HTTP.Port != 17890 || c.HTTP.Host != "127.0.0.1" {
		t.Errorf("http default = %+v, want 127.0.0.1:17890", c.HTTP)
	}
	if c.MQTT.Port != 17891 || c.MQTT.Host != "127.0.0.1" {
		t.Errorf("mqtt default = %+v, want 127.0.0.1:17891", c.MQTT)
	}
	if c.MQTT.Username != "controlhub" || c.MQTT.Password == "" {
		t.Error("mqtt default credentials should be set")
	}
	if c.DataDir != "./data" {
		t.Errorf("data_dir default = %q, want ./data", c.DataDir)
	}
	if c.LogLevel != "info" {
		t.Errorf("log_level default = %q, want info", c.LogLevel)
	}
}

// 写一个临时 YAML 文件并返回路径。
func writeYAML(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	return path
}

func TestLoad_EmptyPath(t *testing.T) {
	// Load("") 用内置默认；Default 会 MkdirAll("./data")，chdir 到 temp 避免污染源码树。
	t.Chdir(t.TempDir())
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\"): %v", err)
	}
	if c.HTTP.Port != 17890 || c.MQTT.Port != 17891 {
		t.Errorf("Load(\"\") should yield defaults, got http=%+v mqtt=%+v", c.HTTP, c.MQTT)
	}
}

func TestLoad_NonexistentPath(t *testing.T) {
	t.Chdir(t.TempDir())
	c, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("Load(nonexistent): %v", err)
	}
	if c.HTTP.Port != 17890 {
		t.Error("nonexistent path should fall back to defaults")
	}
}

func TestLoad_ValidOverride(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data") // Load 会 MkdirAll
	path := writeYAML(t, "http:\n  host: 0.0.0.0\n  port: 8080\nmqtt:\n  host: 10.0.0.1\n  port: 1883\n  username: u\n  password: p\napi_key: my-secret-key\ndata_dir: \""+dataDir+"\"\nlog_level: debug\n")
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HTTP.Host != "0.0.0.0" || c.HTTP.Port != 8080 {
		t.Errorf("http override not applied: %+v", c.HTTP)
	}
	if c.MQTT.Host != "10.0.0.1" || c.MQTT.Port != 1883 || c.MQTT.Username != "u" || c.MQTT.Password != "p" {
		t.Errorf("mqtt override not applied: %+v", c.MQTT)
	}
	if c.APIKey != "my-secret-key" {
		t.Errorf("api_key = %q", c.APIKey)
	}
	if c.LogLevel != "debug" {
		t.Errorf("log_level = %q, want debug", c.LogLevel)
	}
}

func TestLoad_SanitizeFillsDefaults(t *testing.T) {
	// yaml 只给 api_key，http.port=0/mqtt 留空 → sanitize 必须补默认。
	t.Chdir(t.TempDir())
	path := writeYAML(t, "api_key: k\nhttp:\n  port: 0\n")
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HTTP.Port != 17890 {
		t.Errorf("http.port 0 should sanitize to 17890, got %d", c.HTTP.Port)
	}
	if c.MQTT.Port != 17891 {
		t.Errorf("mqtt.port should default to 17891, got %d", c.MQTT.Port)
	}
	if c.LogLevel != "info" {
		t.Errorf("log_level should default to info, got %q", c.LogLevel)
	}
	if c.DataDir == "" {
		t.Error("data_dir should be set after sanitize")
	}
}

func TestLoad_CreatesAndAbsDataDir(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "nested", "deep", "data")
	path := writeYAML(t, "data_dir: \""+dataDir+"\"\n")
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if info, err := os.Stat(dataDir); err != nil || !info.IsDir() {
		t.Errorf("data_dir %q should be created as directory", dataDir)
	}
	if !filepath.IsAbs(c.DataDir) {
		t.Errorf("data_dir should be absolutized, got %q", c.DataDir)
	}
}

func TestLoad_BadYAML(t *testing.T) {
	t.Chdir(t.TempDir())
	path := writeYAML(t, "http: [unterminated\n")
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected parse error for malformed yaml")
	}
}

func TestLoad_NewFieldsDefaults(t *testing.T) {
	// 老配置（无新字段）必须保持旧行为：全开 + 17892
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	os.WriteFile(p, []byte("http:\n  port: 17990\n"), 0o644)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.HTTP.EnableAPI || cfg.HTTP.LanMode {
		t.Errorf("http defaults = enable_api:%v lan_mode:%v, want true/false", cfg.HTTP.EnableAPI, cfg.HTTP.LanMode)
	}
	if !cfg.Pairing.Enabled || cfg.Pairing.Port != 17892 {
		t.Errorf("pairing defaults = %v/%d, want true/17892", cfg.Pairing.Enabled, cfg.Pairing.Port)
	}
	if !cfg.Web.Console || !cfg.Web.Demo || !cfg.Web.Realtime {
		t.Errorf("web defaults = %+v, want all true", cfg.Web)
	}
	if cfg.HTTP.Port != 17990 {
		t.Errorf("explicit http.port = %d, want 17990", cfg.HTTP.Port)
	}
}

func TestLoad_NewFieldsExplicit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	os.WriteFile(p, []byte(`
http: {enable_api: false, lan_mode: true}
pairing: {enabled: false, port: 17992}
web: {console: false, demo: false, realtime: false}
`), 0o644)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.EnableAPI || !cfg.HTTP.LanMode {
		t.Errorf("http = %v/%v, want false/true", cfg.HTTP.EnableAPI, cfg.HTTP.LanMode)
	}
	if cfg.Pairing.Enabled || cfg.Pairing.Port != 17992 {
		t.Errorf("pairing = %v/%d, want false/17992", cfg.Pairing.Enabled, cfg.Pairing.Port)
	}
	if cfg.Web.Console || cfg.Web.Demo || cfg.Web.Realtime {
		t.Errorf("web = %+v, want all false", cfg.Web)
	}
}
