package mqtt

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// InternalUsername 生成凭据时内部 client 使用的固定用户名（非机密；
// 鉴权强度在密码）。设备用户名是 dev_<device_id>，不会冲突。
const InternalUsername = "controlhub"

// GenerateInternalCredential 生成每次启动的随机内部 MQTT 凭据（M1-G3：
// 取代历史固定默认密码）。密码 24 随机字节 hex（48 字符），仅存在于内存：
// broker hook 与内部 client 在同一进程内共享，绝不持久化、绝不写日志。
func GenerateInternalCredential() (username, password string, err error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("gen internal mqtt credential: %w", err)
	}
	return InternalUsername, hex.EncodeToString(raw), nil
}
