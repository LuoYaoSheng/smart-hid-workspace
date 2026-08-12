// Command gen-keys 生成 Ed25519 keypair（CL-1）。
//
// 用法：
//
//	gen-keys <private-key-path> <public-hex-path>
//
// 私钥：64 字节 hex（seed + pub），mode 0600，**勿提交**。
// 公钥：32 字节 hex 字符串，**复制到** smart-hid-controlhub/internal/license/publickey.go。
//
// 用 smart-hid-cloud/scripts/gen-keys.sh 一键调用。
package main

import (
	"fmt"
	"log"
	"os"

	"smart-hid-cloud/pkg/license"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: gen-keys <private-key-path> <public-hex-path>")
		os.Exit(2)
	}
	privPath := os.Args[1]
	pubPath := os.Args[2]

	priv, pub, err := license.GenerateKeypair()
	if err != nil {
		log.Fatalf("gen keypair: %v", err)
	}
	if err := license.SavePrivateKey(privPath, priv); err != nil {
		log.Fatalf("save private: %v", err)
	}
	pubHex := license.PublicKeyHex(pub)
	if err := os.WriteFile(pubPath, []byte(pubHex), 0o644); err != nil {
		log.Fatalf("save public: %v", err)
	}

	fmt.Printf("✔ private key: %s (64 bytes hex, mode 0600)\n", privPath)
	fmt.Printf("✔ public  key: %s\n", pubPath)
	fmt.Printf("  public  hex: %s\n", pubHex)
	fmt.Println()
	fmt.Println("下一步：把上面这行 public hex 复制到")
	fmt.Println("  smart-hid-controlhub/internal/license/publickey.go (CL-3a)")
}
