// Package license: 嵌入 Cloud 签发 License 的对应 Ed25519 公钥（CL-3a）。
//
// 此 hex 字符串由 smart-hid-cloud/scripts/gen-keys.sh 生成（CL-1），
// 从 smart-hid-cloud/keys/public.hex 复制而来。
//
// 重要：换 keypair 时需同步更新此常量 + 重发 ControlHub binary
// （V1 单一公钥，无 key_id/轮换；详见 smart-hid-cloud/docs/license-format.md §8）。

package licmgr

// EmbeddedPublicKeyHex 是 Cloud 用来签 License 的私钥对应的公钥。
// 32 字节 Ed25519 公钥，hex 编码（64 字符）。
const EmbeddedPublicKeyHex = "3795ab7dbadc94804426f66ec5b18f48e484a2417b8c3989f0a124d0a508d1dd"
