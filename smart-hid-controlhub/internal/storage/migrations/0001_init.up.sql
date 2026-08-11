-- Migration 0001: 初始 schema（Phase 1）
-- devices + commands 两表；后续里程碑在更高级 migration 追加。
-- 来源：原 001_init.sql，迁入版本化 migration 体系（CH-P1）。

CREATE TABLE IF NOT EXISTS devices (
    device_id        TEXT PRIMARY KEY,          -- ^HID-[A-Z0-9]{8}$
    boot_id          TEXT NOT NULL DEFAULT '',  -- 当前 boot_id（每次启动变化）
    online           INTEGER NOT NULL DEFAULT 0,-- 0/1
    usb_hid_ready    INTEGER NOT NULL DEFAULT 0,
    firmware         TEXT NOT NULL DEFAULT '',
    last_seen_at     INTEGER NOT NULL DEFAULT 0,-- Unix 秒
    created_at       INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

CREATE TABLE IF NOT EXISTS commands (
    request_id       TEXT PRIMARY KEY,
    device_id        TEXT NOT NULL,
    type             TEXT NOT NULL,             -- keyboard|mouse|system
    action           TEXT NOT NULL,
    ttl_ms           INTEGER NOT NULL,
    status           TEXT NOT NULL,             -- received|executing|executed|rejected|expired|duplicate
    code             INTEGER NOT NULL DEFAULT 0,
    execution_ms     INTEGER,
    created_at       INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    acked_at         INTEGER,
    FOREIGN KEY (device_id) REFERENCES devices(device_id)
);

CREATE INDEX IF NOT EXISTS idx_commands_device ON commands(device_id);
CREATE INDEX IF NOT EXISTS idx_commands_status ON commands(status);
