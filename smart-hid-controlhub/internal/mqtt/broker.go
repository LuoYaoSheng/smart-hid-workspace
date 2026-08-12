// Package mqtt 封装嵌入式 MQTT broker（mochi-mqtt v2）+ ControlHub 作为 client。
//
// Phase 1：仅本机回环 + 静态 user/pass ledger + 粗 ACL。
// Phase 4（CH-P5）：可选切换到 PerDeviceHook，支持每设备凭据 + per-device ACL。
package mqtt

import (
	"database/sql"
	"fmt"
	"log/slog"

	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
)

// Broker 包装 mochi 嵌入式 broker。
type Broker struct {
	server *mochi.Server
	tcp    *listeners.TCP
	log    *slog.Logger
	db     *sql.DB // CH-P5：非空则启用 PerDeviceHook
}

// NewBroker 创建嵌入式 broker（不启动）。
// db 为 nil 时，Start 仍走 Phase 1 的静态 ledger（向后兼容开发期 mock-device）；
// db 非 nil 时，Start 用 PerDeviceHook：hub 用户 + 每设备凭据 + per-device ACL。
func NewBroker(host string, port int, log *slog.Logger) *Broker {
	s := mochi.New(nil)
	tcp := listeners.NewTCP(listeners.Config{
		ID:      "tcp",
		Address: fmt.Sprintf("%s:%d", host, port),
	})
	return &Broker{server: s, tcp: tcp, log: log}
}

// WithDB 设置 device_credentials 表的 *sql.DB 引用，启用 PerDeviceHook。
// 必须在 Start 前调用。
func (b *Broker) WithDB(db *sql.DB) *Broker {
	b.db = db
	return b
}

// Start 启动 broker。
func (b *Broker) Start(username, password string) error {
	if err := b.server.AddListener(b.tcp); err != nil {
		return fmt.Errorf("add tcp listener: %w", err)
	}

	if b.db != nil {
		// CH-P5：PerDeviceHook（hub 用户 + 每设备凭据）
		opts := &PerDeviceHookOptions{DB: b.db, HubUser: username, HubPass: password}
		if err := b.server.AddHook(new(PerDeviceHook), opts); err != nil {
			return fmt.Errorf("add per-device hook: %w", err)
		}
	} else if username != "" {
		// Phase 1 静态 ledger（向后兼容；app.Build 会注入 db，不会走这里）
		ledger := &auth.Ledger{
			Auth: auth.AuthRules{
				{
					Username: auth.RString(username),
					Password: auth.RString(password),
					Allow:    true,
				},
			},
			ACL: auth.ACLRules{
				{Filters: auth.Filters{"smart-hid/#": auth.ReadWrite}},
			},
		}
		if err := b.server.AddHook(new(auth.Hook), &auth.Options{Ledger: ledger}); err != nil {
			return fmt.Errorf("add auth hook: %w", err)
		}
	}

	if err := b.server.Serve(); err != nil {
		return fmt.Errorf("mochi serve: %w", err)
	}
	b.log.Info("mqtt broker started",
		"addr", b.tcp.Address(),
		"auth", username != "",
		"mode", ternary(b.db != nil, "per-device", "static-ledger"))
	return nil
}

// Close 停止 broker。
func (b *Broker) Close() {
	_ = b.server.Close()
}

// NewClient 构造一个 paho MQTT client（用于 ControlHub 自身或 mock-device 连本机 broker）。
func NewClient(host string, port int, clientID, username, password string) pahomqtt.Client {
	opts := pahomqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", host, port))
	opts.SetClientID(clientID)
	if username != "" {
		opts.SetUsername(username)
		opts.SetPassword(password)
	}
	opts.SetAutoReconnect(true)
	return pahomqtt.NewClient(opts)
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
