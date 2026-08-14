// Package api 反垃圾辅助（FB-1）。
//
// 本模块此前无任何限频/客户端 IP 提取代码——本文件是首创。
// V1 采用最简单的进程内滑动窗口（单实例部署够用；多实例时需换共享存储）。
package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ipRateLimiter 进程内每 IP 滑动窗口限频器。
// map 条目在访问时惰性清理（过期时间戳剔除；空键删除），单实例规模无内存压力。
type ipRateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]int64 // ip → 窗口内请求时间戳（unix sec）
	max    int                // 窗口内最大请求数
	window int64              // 窗口长度（秒）
}

func newIPRateLimiter(max int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		hits:   make(map[string][]int64),
		max:    max,
		window: int64(window.Seconds()),
	}
}

// allow 记录一次请求并判断是否放行。
func (l *ipRateLimiter) allow(ip string, now int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	// 惰性清理：只保留窗口内的时间戳
	kept := l.hits[ip][:0]
	for _, ts := range l.hits[ip] {
		if now-ts < l.window {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= l.max {
		l.hits[ip] = kept
		return false
	}
	l.hits[ip] = append(kept, now)
	return true
}

// clientIP 提取客户端 IP：优先 X-Forwarded-For 首值（反代场景），否则 RemoteAddr 的 host 部分。
//
// ⚠ 信任模型（docs/feedback.md §威胁模型）：直连/无反代时 RemoteAddr 不可伪造；
// 有反代时攻击者可自带伪造 XFF 头绕过限频 —— 生产部署必须让反代覆盖该头
// （nginx: proxy_set_header X-Forwarded-For $remote_addr）。Phase 7 再上可信代理配置。
func clientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		// 取第一个（最原始客户端）；"client, proxy1, proxy2" 格式
		if i := strings.IndexByte(xf, ','); i >= 0 {
			xf = xf[:i]
		}
		return strings.TrimSpace(xf)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
