// Package mqtt 封装嵌入式 MQTT broker（mochi-mqtt v2）+ ControlHub 作为 client。
// Phase 1：仅本机回环 + 用户名/密码（用内置 auth ledger）；不做细粒度 ACL（Phase 4 接入）。
package mqtt

import (
	"fmt"
	"log/slog"

	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
)

// Broker 包装 mochi 嵌入式 broker。
type Broker struct {
	server *mochi.Server
	tcp    *listeners.TCP
	log    *slog.Logger
}

// NewBroker 创建嵌入式 broker（不启动）。
// Phase 1：若 username 为空，则允许匿名（开发期 mock-device 可省凭证）；
//          否则注册 auth ledger 强制 user/pass 匹配 + 默认 ACL 读写。
func NewBroker(host string, port int, username, password string, log *slog.Logger) *Broker {
	s := mochi.New(nil)

	tcp := listeners.NewTCP(listeners.Config{
		ID:      "tcp",
		Address: fmt.Sprintf("%s:%d", host, port),
	})

	return &Broker{server: s, tcp: tcp, log: log}
}

// Start 启动 broker。
func (b *Broker) Start(username, password string) error {
	if err := b.server.AddListener(b.tcp); err != nil {
		return fmt.Errorf("add tcp listener: %w", err)
	}

	// 鉴权：username 非空则强制；空则允许匿名（开发期）
	if username != "" {
		ledger := &auth.Ledger{
			Auth: auth.AuthRules{
				{
					Username: auth.RString(username),
					Password: auth.RString(password),
					Allow:    true,
				},
			},
			// 默认 ACL：已认证用户读写 smart-hid/#
			ACL: auth.ACLRules{
				{
					Filters: auth.Filters{
						"smart-hid/#": auth.ReadWrite,
					},
				},
			},
		}
		opts := &auth.Options{Ledger: ledger}
		if err := b.server.AddHook(new(auth.Hook), opts); err != nil {
			return fmt.Errorf("add auth hook: %w", err)
		}
	}
	// username 为空时不加 auth hook → mochi 默认允许匿名

	if err := b.server.Serve(); err != nil {
		return fmt.Errorf("mochi serve: %w", err)
	}
	b.log.Info("mqtt broker started", "addr", b.tcp.Address(), "auth", username != "")
	return nil
}

// Close 停止 broker。
func (b *Broker) Close() {
	_ = b.server.Close()
}

// 引用 packets 包避免编译期 "imported and not used"（保留以便后续扩展 hook）。
var _ packets.Packet = packets.Packet{}

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
