// Package netaddr 解决 ControlHub 的 MQTT 网络地址三语义拆分（M1-G3）：
//
//	bind_host       embedded broker 监听地址（默认 0.0.0.0，LAN 设备可达）
//	internal_host   ControlHub 自身连本机 broker 的地址（由 bind_host 推导，非用户配置）
//	advertise_host  返回给 ESP32 的 broker 地址（resolver 解析，绝不返回环回/通配）
//
// 解析优先级（docs/current/HARDENING_BACKLOG M1-G3）：
//  1. 用户显式配置 mqtt.advertise_host
//  2. 设备 pairing 请求实际到达的本地地址（http.LocalAddrContextKey）
//  3. 向 peer 做 UDP 拨号推导出口 IP（不发真实流量）
//  4. 唯一可用 LAN IPv4（过滤 loopback / link-local / 容器虚拟网卡）
//  5. 无法可靠判断 → 明确失败（禁止偷返回 127.0.0.1，禁止随机选第一块网卡）
//
// 唯一例外：peer 本身是环回地址（本机 mock-device / 本机测试）时，环回
// advertise 是唯一正确答案——真实 ESP32 不可能从环回地址发起 pairing。
package netaddr

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// 虚拟网卡名前缀：这些接口上的地址不参与"唯一可用 LAN IPv4"候选
// （Docker 桥 / veth / libvirt 桥是宿主机内部拓扑，不是设备可达网络）。
// Tailscale/VPN（utun*）保留为候选——它是合法的可达路径，交给"唯一性"规则裁决。
var virtualInterfacePrefixes = []string{"docker", "veth", "br-", "virbr", "podman", "vibr"}

// InterfaceSnapshot 是地址级网卡快照。
// 不直接用 net.Interface 是因为其 Addrs() 走系统调用，无法注入测试数据。
type InterfaceSnapshot struct {
	Name  string
	Up    bool
	Addrs []net.IPNet
}

// SnapshotLister 返回网卡快照；生产实现见 realInterfaces，测试注入内存数据。
type SnapshotLister func() ([]InterfaceSnapshot, error)

// Dialer 向 peer 推导出口本地 IP；生产用 UDP 拨号（不发包），测试注入。
type Dialer func(peer net.IP) (net.IP, error)

// Resolver 解析 advertise host。零值不可用，用 New。
type Resolver struct {
	explicit  string
	snapshots SnapshotLister
	dial      Dialer
}

// New 创建 resolver。explicit 为 mqtt.advertise_host（空 = 自动解析）。
func New(explicit string) *Resolver {
	return &Resolver{
		explicit:  strings.TrimSpace(explicit),
		snapshots: realInterfaces,
		dial:      dialUDPRoute,
	}
}

// WithSnapshots 替换网卡快照（测试注入）。
func (r *Resolver) WithSnapshots(f SnapshotLister) *Resolver {
	r.snapshots = f
	return r
}

// WithDialer 替换出口推导（测试注入）。
func (r *Resolver) WithDialer(f Dialer) *Resolver {
	r.dial = f
	return r
}

// Resolve 按 1→5 优先级解析 advertise host。
// localAddr 是设备/浏览器请求实际到达的本地地址（可为 nil）；peer 是对端 IP（可为 nil）。
func (r *Resolver) Resolve(localAddr net.Addr, peer net.IP) (string, error) {
	// 1. 显式配置（Load 时已校验，这里防御性再验）
	if r.explicit != "" {
		if err := ValidateAdvertiseHost(r.explicit); err != nil {
			return "", fmt.Errorf("mqtt.advertise_host invalid: %w", err)
		}
		return r.explicit, nil
	}

	// 2. 请求实际到达的本地地址——peer 路径的精确答案
	if localAddr != nil {
		if host, _, err := net.SplitHostPort(localAddr.String()); err == nil {
			ip := net.ParseIP(host)
			if ip != nil && usableAdvertiseIP(ip, peer) {
				return ip.String(), nil
			}
		}
	}

	// 3. 向 peer 推导出口 IP（UDP 拨号不发包）
	if peer != nil && !peer.IsLoopback() {
		if ip, err := r.dial(peer); err == nil && ip != nil && usableAdvertiseIP(ip, peer) {
			return ip.String(), nil
		}
	}

	// 4. 唯一可用 LAN IPv4
	candidates := r.lanCandidates()
	if len(candidates) == 1 {
		return candidates[0].ip.String(), nil
	}

	// 5. 明确失败（0 个或多个候选都失败——禁止猜）
	if len(candidates) == 0 {
		return "", fmt.Errorf("no usable LAN IPv4 address found; set mqtt.advertise_host explicitly")
	}
	var parts []string
	for _, c := range candidates {
		parts = append(parts, fmt.Sprintf("%s=%s", c.name, c.ip))
	}
	return "", fmt.Errorf("multiple LAN IPv4 candidates (%s); set mqtt.advertise_host explicitly", strings.Join(parts, ", "))
}

type lanCandidate struct {
	name string
	ip   net.IP
}

// lanCandidates 过滤出可用 LAN IPv4：跳过 down / loopback / link-local /
// 容器虚拟网卡；IPv6 不参与（MQTT advertised host 走 IPv4，IPv6 留给显式配置）。
func (r *Resolver) lanCandidates() []lanCandidate {
	snapshots, err := r.snapshots()
	if err != nil {
		return nil
	}
	var out []lanCandidate
	for _, s := range snapshots {
		if !s.Up || isVirtualInterface(s.Name) {
			continue
		}
		for _, ipnet := range s.Addrs {
			v4 := ipnet.IP.To4()
			if v4 == nil || !usableAdvertiseIP(v4, nil) {
				continue
			}
			out = append(out, lanCandidate{name: s.Name, ip: v4})
		}
	}
	return out
}

func isVirtualInterface(name string) bool {
	for _, p := range virtualInterfacePrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// usableAdvertiseIP 判断 ip 是否可以作为返回给设备的 advertise 地址。
// peer 为 nil 时按通用规则（环回/通配/链路本地一律不可用）；
// peer 是环回（本机 mock/测试）时环回可用——这是唯一允许环回的场景。
func usableAdvertiseIP(ip net.IP, peer net.IP) bool {
	if ip.IsUnspecified() { // 0.0.0.0 / ::
		return false
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() { // 169.254.x / fe80::
		return false
	}
	if ip.IsLoopback() {
		return peer != nil && peer.IsLoopback()
	}
	return true
}

// ValidateAdvertiseHost 校验显式配置的 advertise host：
// 拒绝 localhost / 环回 / 通配 / 未指定 / 链路本地（HARDENING_BACKLOG §8：禁止返回给设备）。
// 允许 IPv4 / IPv6 单播 / DNS 主机名。
func ValidateAdvertiseHost(host string) error {
	h := strings.TrimSpace(host)
	if h == "" {
		return fmt.Errorf("empty advertise host")
	}
	if strings.EqualFold(h, "localhost") {
		return fmt.Errorf("localhost is not a device-reachable address")
	}
	if ip := net.ParseIP(h); ip != nil {
		if ip.IsLoopback() {
			return fmt.Errorf("%s is loopback; ESP32 would connect to itself", h)
		}
		if ip.IsUnspecified() {
			return fmt.Errorf("%s is unspecified; not a connectable target", h)
		}
		if ip.IsLinkLocalUnicast() {
			return fmt.Errorf("%s is link-local; not routable to devices", h)
		}
		return nil
	}
	return nil // DNS 主机名，交给设备解析
}

// InternalConnectHost 由 bind_host 推导 ControlHub 内部 client 连接地址：
// 通配 bind → 环回（broker 在本机，环回是最稳的内部路径）；
// 具体 IP/主机名 bind → 原值。
func InternalConnectHost(bindHost string) string {
	h := strings.TrimSpace(bindHost)
	if h == "" {
		return "127.0.0.1"
	}
	if ip := net.ParseIP(h); ip != nil && ip.IsUnspecified() {
		if ip.To4() == nil {
			return "::1" // 通配 IPv6
		}
		return "127.0.0.1"
	}
	return h
}

// realInterfaces 生产实现：net.Interfaces + Addrs 系统调用 → 快照。
func realInterfaces() ([]InterfaceSnapshot, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make([]InterfaceSnapshot, 0, len(ifaces))
	for _, ifc := range ifaces {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		snap := InterfaceSnapshot{Name: ifc.Name, Up: ifc.Flags&net.FlagUp != 0}
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok {
				snap.Addrs = append(snap.Addrs, *ipnet)
			}
		}
		out = append(out, snap)
	}
	return out, nil
}

// dialUDPRoute 用 UDP 拨号（不发包）拿到朝向 peer 的本地出口 IP。
func dialUDPRoute(peer net.IP) (net.IP, error) {
	d := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := d.Dial("udp", net.JoinHostPort(peer.String(), "9"))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	local, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || local.IP == nil {
		return nil, fmt.Errorf("no local addr")
	}
	return local.IP, nil
}
