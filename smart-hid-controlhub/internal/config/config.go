// Package config 加载 ControlHub 配置。
// YAML 文件（可选）覆盖在内置默认值之上；不支持环境变量覆盖。
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"smart-hid-controlhub/internal/netaddr"

	"gopkg.in/yaml.v3"
)

// Config 是 ControlHub 运行配置。
type Config struct {
	HTTP     HTTPConfig     `yaml:"http"`
	MQTT     MQTTConfig     `yaml:"mqtt"`
	Pairing  PairingConfig  `yaml:"pairing"`
	Web      WebConfig      `yaml:"web"`
	APIKey   string         `yaml:"api_key"`
	DataDir  string         `yaml:"data_dir"`
	LogLevel string         `yaml:"log_level"`
}

type HTTPConfig struct {
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	LanMode   bool   `yaml:"lan_mode"`   // 启动即监听 0.0.0.0（控制台运行时开关持久化后优先）
	EnableAPI bool   `yaml:"enable_api"` // false = 不注册 /api/v1（纯静态模式）
}

// MQTTConfig 是 MQTT 网络模型（M1-G3 拆分，见 internal/netaddr）：
//
//	bind_host       embedded broker 监听地址（默认 0.0.0.0，LAN 设备可达）
//	advertise_host  返回给设备的 broker 地址（空 = 按请求路径自动解析）
//	username/password 内部 client 凭据；两者必须成对配置，都留空 = 每次启动随机生成
//
// legacy：G3 之前的 mqtt.host 一字段三用，Load 时按下述规则迁移并打一次
// deprecated 警告（见 migrateLegacyMQTTHost）。
type MQTTConfig struct {
	BindHost      string `yaml:"bind_host"`
	AdvertiseHost string `yaml:"advertise_host"`
	Port          int    `yaml:"port"`
	Username      string `yaml:"username"`
	Password      string `yaml:"password"`

	// Host 是 legacy 字段（只写不读）：yaml 里出现 mqtt.host 时由 Load 迁移。
	Host string `yaml:"host"`

	// LegacyHostUsed 标记本次加载迁移过 legacy mqtt.host（供 app 打一次性警告）。
	LegacyHostUsed bool `yaml:"-"`
}

// PairingConfig 设备侧配对服务（原端口 17892 硬编码，现可配）。
type PairingConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
}

// WebConfig 内置 Web 界面的各页面开关。
type WebConfig struct {
	Console  bool `yaml:"console"`  // 控制台 + API 测试页
	Demo     bool `yaml:"demo"`     // 模拟键鼠演示台（demo.html）
	Realtime bool `yaml:"realtime"` // WebSocket 实时事件通道（/api/v1/realtime）
}

// Default 返回内置默认配置（无 config.yaml 时使用）。
// MQTT 默认：bind 0.0.0.0（设备经 LAN 接入是产品主场景；broker 有
// per-device 凭据 + ACL 保护），内部凭据留空 = 每次启动随机生成（不再有
// 固定默认密码）。
func Default() *Config {
	return &Config{
		HTTP:     HTTPConfig{Host: "127.0.0.1", Port: 17890, LanMode: false, EnableAPI: true},
		MQTT:     MQTTConfig{BindHost: "0.0.0.0", Port: 17891},
		Pairing:  PairingConfig{Enabled: true, Port: 17892},
		Web:      WebConfig{Console: true, Demo: true, Realtime: true},
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
	if cfg.Pairing.Port == 0 {
		cfg.Pairing.Port = 17892
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	// legacy mqtt.host 迁移（M1-G3 之前的一字段三用）
	if err := migrateLegacyMQTTHost(&cfg.MQTT); err != nil {
		return nil, err
	}
	if cfg.MQTT.BindHost == "" {
		cfg.MQTT.BindHost = "0.0.0.0"
	}

	// advertise_host 显式配置时启动即校验（拒绝环回/通配等设备不可达地址）
	if cfg.MQTT.AdvertiseHost != "" {
		if err := netaddr.ValidateAdvertiseHost(cfg.MQTT.AdvertiseHost); err != nil {
			return nil, fmt.Errorf("mqtt.advertise_host: %w", err)
		}
	}

	// 内部凭据必须成对：都空 = 随机生成（app 层处理）；只配一半 = 配置错误
	if (cfg.MQTT.Username == "") != (cfg.MQTT.Password == "") {
		return nil, fmt.Errorf("mqtt.username and mqtt.password must be set together (or both empty for a per-boot random credential)")
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

// migrateLegacyMQTTHost 迁移 G3 之前的 mqtt.host（bind / internal connect /
// advertised 三用）。Default() 的 Host 为空，因此 unmarshal 后非空即用户显式写了
// legacy 字段。规则（spec M1-G3 §12）：
//
//	环回（127.0.0.1/localhost/::1）→ bind 兼容读取；advertise 不允许直接使用（留空自动解析）
//	通配（0.0.0.0/::）            → bind 兼容读取；advertise 留空自动解析
//	具体 IP / 主机名               → 同时作为 bind 与 advertise 候选
//
// 迁移后置空 Host，防止三处读值再分叉；LegacyHostUsed 供 app 打一次 deprecated 警告。
func migrateLegacyMQTTHost(m *MQTTConfig) error {
	if m.Host == "" {
		return nil
	}
	m.LegacyHostUsed = true
	host := m.Host
	m.Host = ""

	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsUnspecified() {
			m.BindHost = host
			return nil
		}
	}
	if strings.EqualFold(host, "localhost") {
		m.BindHost = "127.0.0.1"
		return nil
	}
	// 具体 LAN IP / 主机名：bind + advertise 都用它
	m.BindHost = host
	if m.AdvertiseHost == "" {
		m.AdvertiseHost = host
	}
	return nil
}
