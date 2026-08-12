// PerDeviceHook 替代静态 Ledger 的鉴权 hook（CH-P5）。
//
// 支持两类 MQTT 客户端：
//  1. ControlHub 自身 — 用 cfg.MQTT.Username/Password，全 topic 读写
//     （订阅 smart-hid/v1/devices/+/ack、+/status，发布 +/command）。
//  2. 设备 — 用 dev_<device_id> 用户名 + 32 字节随机密码（配对时签发），
//     密码 hash 存 device_credentials 表；per-device ACL 仅允许自己的 topic。
//
// 设计源：docs/05 §6（命令引擎 pipeline 之前）+ §4 端口约定 +
// 验收清单 A5（MQTT 需认证）+ A7（每设备 Topic ACL）。
//
// 安全：密码 hash 用 SHA-256，比较用 crypto/subtle.ConstantTimeCompare
// （device-side 凭据每设备独立，撤销 = device_credentials.revoked_at 置值）。
package mqtt

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"strings"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"
)

const (
	// DeviceUserPrefix 配对签发的设备用户名前缀。完整用户名 = dev_<device_id>。
	DeviceUserPrefix = "dev_"
	// DeviceTopicPrefix 设备级 topic 前缀，完整 topic = smart-hid/v1/devices/<device_id>/<sub>。
	DeviceTopicPrefix = "smart-hid/v1/devices/"
)

// PerDeviceHookOptions 是 AddHook 的 config。
type PerDeviceHookOptions struct {
	DB      *sql.DB
	HubUser string // cfg.MQTT.Username
	HubPass string // cfg.MQTT.Password
}

// PerDeviceHook 实现 mochi HookBase，按用户类型分发鉴权。
type PerDeviceHook struct {
	mqtt.HookBase
	db      *sql.DB
	hubUser []byte
	hubPass []byte
}

// ID 返回 hook 唯一标识。
func (h *PerDeviceHook) ID() string { return "per-device-auth" }

// Provides 声明 hook 实现的能力：连接鉴权 + ACL 检查。
func (h *PerDeviceHook) Provides(b byte) bool {
	return bytes.Contains([]byte{
		mqtt.OnConnectAuthenticate,
		mqtt.OnACLCheck,
	}, []byte{b})
}

// Init 接收 PerDeviceHookOptions。
func (h *PerDeviceHook) Init(config any) error {
	opts, ok := config.(*PerDeviceHookOptions)
	if !ok || opts == nil {
		return mqtt.ErrInvalidConfigType
	}
	h.db = opts.DB
	h.hubUser = []byte(opts.HubUser)
	h.hubPass = []byte(opts.HubPass)
	if h.Log != nil {
		h.Log.Info("per-device auth hook loaded",
			"hub_user_set", len(h.hubUser) > 0)
	}
	return nil
}

// OnConnectAuthenticate 返回 true 表示允许连接。
func (h *PerDeviceHook) OnConnectAuthenticate(cl *mqtt.Client, pk packets.Packet) bool {
	user := string(pk.Connect.Username)
	pass := pk.Connect.Password

	// 1. ControlHub 自身
	if len(h.hubUser) > 0 && user == string(h.hubUser) {
		if subtle.ConstantTimeCompare(pass, h.hubPass) == 1 {
			return true
		}
		if h.Log != nil {
			h.Log.Warn("hub auth failed (bad pass)", "user", user)
		}
		return false
	}

	// 2. 设备用户 dev_<device_id>
	if strings.HasPrefix(user, DeviceUserPrefix) {
		deviceID := strings.TrimPrefix(user, DeviceUserPrefix)
		return h.authDevice(deviceID, pass)
	}

	if h.Log != nil {
		h.Log.Warn("auth rejected (unknown user class)", "user", user)
	}
	return false
}

func (h *PerDeviceHook) authDevice(deviceID string, pass []byte) bool {
	var hash string
	err := h.db.QueryRow(
		`SELECT mqtt_credential_hash FROM device_credentials
		 WHERE device_id = ? AND revoked_at IS NULL`,
		deviceID,
	).Scan(&hash)
	if err != nil {
		if h.Log != nil {
			h.Log.Warn("device credential missing",
				"device_id", deviceID, "err", err)
		}
		return false
	}
	sum := sha256.Sum256(pass)
	passHash := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(passHash), []byte(hash)) == 1 {
		return true
	}
	if h.Log != nil {
		h.Log.Warn("device auth failed (bad pass)", "device_id", deviceID)
	}
	return false
}

// OnACLCheck 返回 true 表示允许对该 topic 进行 read/write。
func (h *PerDeviceHook) OnACLCheck(cl *mqtt.Client, topic string, write bool) bool {
	user := string(cl.Properties.Username)

	// 1. ControlHub 自身：全 topic 读写
	if len(h.hubUser) > 0 && user == string(h.hubUser) {
		return strings.HasPrefix(topic, "smart-hid/")
	}

	// 2. 设备：仅允许 smart-hid/v1/devices/<device_id>/* （自己的）
	if strings.HasPrefix(user, DeviceUserPrefix) {
		deviceID := strings.TrimPrefix(user, DeviceUserPrefix)
		allowed := DeviceTopicPrefix + deviceID + "/"
		return strings.HasPrefix(topic, allowed)
	}

	return false
}
