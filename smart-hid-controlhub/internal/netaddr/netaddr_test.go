package netaddr

import (
	"fmt"
	"net"
	"strings"
	"testing"
)

// fakeAddr 实现 net.Addr，模拟 http.LocalAddrContextKey 拿到的连接本地地址。
type fakeAddr struct{ host string }

func (f fakeAddr) Network() string { return "tcp" }
func (f fakeAddr) String() string  { return f.host + ":12345" }

// snap 构造一张网卡快照。cidrs 形如 "192.168.1.20/24"。
func snap(name string, up bool, cidrs ...string) InterfaceSnapshot {
	s := InterfaceSnapshot{Name: name, Up: up}
	for _, c := range cidrs {
		ip, ipnet, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		ipnet.IP = ip // ParseCIDR 的 IPNet.IP 是网络地址，换回主机地址
		s.Addrs = append(s.Addrs, *ipnet)
	}
	return s
}

func snaps(list ...InterfaceSnapshot) SnapshotLister {
	return func() ([]InterfaceSnapshot, error) { return list, nil }
}

func mustIP(s string) net.IP {
	ip := net.ParseIP(s)
	if ip == nil {
		panic("bad ip " + s)
	}
	return ip
}

// TestExplicitAdvertiseHostWins 显式配置最优先（即使请求路径有更精确的地址）。
func TestExplicitAdvertiseHost(t *testing.T) {
	r := New("192.168.1.8").
		WithSnapshots(snaps(snap("en0", true, "10.0.0.5/24"))).
		WithDialer(func(net.IP) (net.IP, error) { return mustIP("10.0.0.5"), nil })
	got, err := r.Resolve(fakeAddr{"10.0.0.5"}, mustIP("10.0.0.99"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "192.168.1.8" {
		t.Fatalf("explicit should win, got %s", got)
	}
}

// TestAdvertiseHostFromPairingRoute 设备请求到达的本地地址是精确答案：
// 多网卡（LAN + Tailscale + docker0）下直接选中请求路径上的 LAN IP。
func TestAdvertiseHostFromPairingRoute(t *testing.T) {
	r := New("").WithSnapshots(snaps(
		snap("en0", true, "192.168.1.20/24"),
		snap("utun4", true, "100.101.1.5/32"),   // Tailscale
		snap("docker0", false, "172.17.0.1/16"), // down 的 docker 桥
	))
	// 设备 192.168.1.50 连到本机 192.168.1.20:17892
	got, err := r.Resolve(fakeAddr{"192.168.1.20"}, mustIP("192.168.1.50"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "192.168.1.20" {
		t.Fatalf("want 192.168.1.20 (request path), got %s", got)
	}
}

// TestAdvertiseHostRouteDialFallback 无 LocalAddr 时用出口推导。
func TestAdvertiseHostRouteDialFallback(t *testing.T) {
	r := New("").
		WithSnapshots(snaps(snap("en0", true, "192.168.1.20/24"))).
		WithDialer(func(peer net.IP) (net.IP, error) {
			if !peer.Equal(mustIP("192.168.1.50")) {
				return nil, fmt.Errorf("unexpected peer %s", peer)
			}
			return mustIP("192.168.1.20"), nil
		})
	got, err := r.Resolve(nil, mustIP("192.168.1.50"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "192.168.1.20" {
		t.Fatalf("want route-derived 192.168.1.20, got %s", got)
	}
}

// TestAdvertiseHostLoopbackPeer 本机 mock/测试场景：peer 环回 → 环回可用。
func TestAdvertiseHostLoopbackPeer(t *testing.T) {
	r := New("").WithSnapshots(snaps())
	got, err := r.Resolve(fakeAddr{"127.0.0.1"}, mustIP("127.0.0.1"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "127.0.0.1" {
		t.Fatalf("loopback peer should get loopback advertise, got %s", got)
	}
}

// TestAdvertiseHostMultiNICSelection 唯一可用 LAN IPv4 才允许 fallback：
// LAN + Tailscale（两个候选）→ 失败并给出候选清单；LAN + docker → 唯一 → 选中 LAN。
func TestAdvertiseHostMultiNICSelection(t *testing.T) {
	// LAN + Tailscale：无法可靠判断 → 明确失败
	r := New("").WithSnapshots(snaps(
		snap("en0", true, "192.168.1.20/24"),
		snap("utun4", true, "100.101.1.5/32"),
	)).WithDialer(func(net.IP) (net.IP, error) { return nil, fmt.Errorf("no route") })
	_, err := r.Resolve(nil, nil)
	if err == nil {
		t.Fatal("LAN+Tailscale ambiguity should fail, not guess")
	}
	if !strings.Contains(err.Error(), "multiple LAN IPv4 candidates") ||
		!strings.Contains(err.Error(), "192.168.1.20") {
		t.Fatalf("error should list candidates, got: %v", err)
	}

	// LAN + docker0：docker 是虚拟网卡不计入候选 → 唯一 LAN → 选中
	r2 := New("").WithSnapshots(snaps(
		snap("en0", true, "192.168.1.20/24"),
		snap("docker0", true, "172.17.0.1/16"),
		snap("veth0a1b2c3", true, "172.18.0.2/16"),
	)).WithDialer(func(net.IP) (net.IP, error) { return nil, fmt.Errorf("no route") })
	got, err := r2.Resolve(nil, nil)
	if err != nil {
		t.Fatalf("resolve unique LAN: %v", err)
	}
	if got != "192.168.1.20" {
		t.Fatalf("want 192.168.1.20, got %s", got)
	}

	// 只有环回 + link-local：零候选 → 明确失败（禁止回退 127.0.0.1）
	r3 := New("").WithSnapshots(snaps(
		snap("lo0", true, "127.0.0.1/8"),
		snap("en1", true, "169.254.7.9/16"),
	)).WithDialer(func(net.IP) (net.IP, error) { return nil, fmt.Errorf("no route") })
	_, err = r3.Resolve(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "no usable LAN IPv4") {
		t.Fatalf("zero candidates should fail explicitly, got: %v", err)
	}
}

// TestAdvertiseHostWildcardLocalRejected LocalAddr 是通配地址（不应出现，防御）→ 跳过。
func TestAdvertiseHostWildcardLocalRejected(t *testing.T) {
	r := New("").
		WithSnapshots(snaps(snap("en0", true, "192.168.1.20/24"))).
		WithDialer(func(net.IP) (net.IP, error) { return mustIP("192.168.1.20"), nil })
	got, err := r.Resolve(fakeAddr{"0.0.0.0"}, mustIP("192.168.1.50"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "192.168.1.20" {
		t.Fatalf("wildcard local should be skipped, got %s", got)
	}
}

// TestRejectLoopbackAdvertiseHost 显式环回 / localhost 一律拒绝。
func TestRejectLoopbackAdvertiseHost(t *testing.T) {
	for _, bad := range []string{"127.0.0.1", "::1", "localhost", "LOCALHOST"} {
		if err := ValidateAdvertiseHost(bad); err == nil {
			t.Fatalf("%s should be rejected", bad)
		}
	}
	r := New("127.0.0.1")
	if _, err := r.Resolve(nil, nil); err == nil {
		t.Fatal("explicit loopback must fail resolve")
	}
}

// TestRejectWildcardAdvertiseHost 通配地址拒绝。
func TestRejectWildcardAdvertiseHost(t *testing.T) {
	for _, bad := range []string{"0.0.0.0", "::"} {
		if err := ValidateAdvertiseHost(bad); err == nil {
			t.Fatalf("%s should be rejected", bad)
		}
	}
}

// TestValidateAdvertiseHostAccepts 合法值：IPv4 / IPv6 全局单播 / 主机名。
func TestValidateAdvertiseHostAccepts(t *testing.T) {
	for _, ok := range []string{"192.168.1.8", "10.0.0.5", "fd00::1", "hub.lan.example"} {
		if err := ValidateAdvertiseHost(ok); err != nil {
			t.Fatalf("%s should be accepted: %v", ok, err)
		}
	}
}

// TestInternalConnectAddressForWildcardBind 内部连接地址推导：
// 通配 → 环回；具体 IP / 主机名 / 环回 bind → 原值。
func TestInternalConnectAddressForWildcardBind(t *testing.T) {
	cases := []struct{ bind, want string }{
		{"0.0.0.0", "127.0.0.1"},
		{"::", "::1"},
		{"192.168.1.20", "192.168.1.20"},
		{"127.0.0.1", "127.0.0.1"},
		{"hub.local", "hub.local"},
		{"", "127.0.0.1"},
	}
	for _, c := range cases {
		if got := InternalConnectHost(c.bind); got != c.want {
			t.Fatalf("InternalConnectHost(%q) = %q, want %q", c.bind, got, c.want)
		}
	}
}
