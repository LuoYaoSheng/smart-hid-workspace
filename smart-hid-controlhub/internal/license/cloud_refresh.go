// Package licmgr: 在线刷新辅助（CL-6c）。
//
// 通过 CloudFetcher 接口拉取最新签名 License 并覆盖导入（续期后自动续期）。
// 用接口而非直接依赖 internal/cloud，保持本包"本地 license 管理"的纯粹性
// （依赖反转：*cloud.Client 实现此接口）。
package licmgr

import (
	"context"
	"log/slog"
	"time"
)

// CloudFetcher 抽象"从 Cloud 拉一个 license"的能力。*cloud.Client 实现此接口。
type CloudFetcher interface {
	RefreshLicense(ctx context.Context, licenseID string) ([]byte, error)
}

// RefreshResult 记录批量刷新每条 license 的结果。
type RefreshResult struct {
	LicenseID string `json:"license_id"`
	DeviceID  string `json:"device_id"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

// RefreshFromCloud 拉取并覆盖导入一个 license（CL-6c）。
// 用于按设备手动刷新（API /license/refresh?device_id=X）。
func (m *Manager) RefreshFromCloud(ctx context.Context, fetcher CloudFetcher, licenseID, deviceID string) error {
	raw, err := fetcher.RefreshLicense(ctx, licenseID)
	if err != nil {
		return err
	}
	_, err = m.Import(raw, deviceID) // Import 含 VerifyFull + upsert
	return err
}

// RefreshAllFromCloud 批量刷新所有本地 license（CL-6c）。
// 用于后台自动刷新循环 + 托盘"刷新 License"。失败的单条不中断整体。
func (m *Manager) RefreshAllFromCloud(ctx context.Context, fetcher CloudFetcher) []RefreshResult {
	list, err := m.ListAll()
	if err != nil {
		return nil
	}
	var out []RefreshResult
	for _, info := range list {
		err := m.RefreshFromCloud(ctx, fetcher, info.LicenseID, info.DeviceID)
		r := RefreshResult{LicenseID: info.LicenseID, DeviceID: info.DeviceID}
		if err != nil {
			r.Error = err.Error()
		} else {
			r.OK = true
		}
		out = append(out, r)
	}
	return out
}

// Refresher 是后台 best-effort 刷新循环（CL-6c）。
// 启动后延迟 startupDelay 跑一次，之后每 interval 跑一次。
// 网络失败只 log，不影响本地 License（local-first）。
type Refresher struct {
	mgr          *Manager
	fetcher      CloudFetcher
	log          *slog.Logger
	interval     time.Duration
	startupDelay time.Duration
}

// NewRefresher 构造后台刷新器。interval<=0 时用默认 6h。
func NewRefresher(mgr *Manager, fetcher CloudFetcher, log *slog.Logger, interval time.Duration) *Refresher {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	return &Refresher{
		mgr: mgr, fetcher: fetcher, log: log,
		interval:     interval,
		startupDelay: 30 * time.Second,
	}
}

// Start 启动后台循环（阻塞至 ctx 取消）。
func (rf *Refresher) Start(ctx context.Context) {
	go func() {
		rf.log.Info("license refresher started", "interval", rf.interval, "startup_delay", rf.startupDelay)
		// 启动后延迟一次（避免与启动峰值抢资源）
		timer := time.NewTimer(rf.startupDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return
		}
		rf.refreshAll(ctx)

		ticker := time.NewTicker(rf.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rf.refreshAll(ctx)
			case <-ctx.Done():
				rf.log.Info("license refresher stopped")
				return
			}
		}
	}()
}

// RefreshAllNow 立即刷新全部（托盘/API 手动触发用）。返回 (成功数, 失败数)。
func (rf *Refresher) RefreshAllNow() (ok, failed int) {
	results := rf.mgr.RefreshAllFromCloud(context.Background(), rf.fetcher)
	for _, r := range results {
		if r.OK {
			ok++
		} else {
			failed++
		}
	}
	rf.log.Info("license refresh (manual)", "ok", ok, "failed", failed)
	return ok, failed
}

func (rf *Refresher) refreshAll(ctx context.Context) {
	results := rf.mgr.RefreshAllFromCloud(ctx, rf.fetcher)
	ok, failed := 0, 0
	for _, r := range results {
		if r.OK {
			ok++
		} else {
			failed++
		}
	}
	if failed > 0 {
		rf.log.Warn("license refresh (auto) completed with failures",
			"ok", ok, "failed", failed, "note", "offline or cloud unreachable; local licenses still valid")
	} else {
		rf.log.Info("license refresh (auto) ok", "refreshed", ok)
	}
}
