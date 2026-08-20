-- Migration 0002: Phase 4/5 schema 扩展
-- 设计源：docs/05_CONTROLHUB_DETAIL_DESIGN_V1.0.md §9（ControlHub SQLite schema 12 张表全集）
-- 命名分歧说明：设计文档用 `command_history`，本仓库保留 Phase 1 已有的 `commands` 表名
-- 不重命名，避免动 Engine/HandleAck/api.handlers 的全部引用；在 README 注明。
--
-- 本 migration 引入的表/列：
--   app_meta            键值配置/状态
--   settings            用户可改的运行时配置（lan_mode_enabled, ...）
--   devices 新列        device_name / paired_at / is_paired / machine_anchor
--   device_credentials  配对成功后签发的每设备 MQTT 凭据
--   pairing_sessions    配对会话生命周期
--   api_keys            API key 持久化（哈希存储）
--   security_events     安全审计日志
--
-- 注意：SQLite ALTER TABLE ADD COLUMN 不支持 IF NOT EXISTS；
-- 但 RunMigrations 通过 schema_migrations 表保证每个 migration 只执行一次，
-- 因此 0002 不会被重复 apply，ADD COLUMN 是安全的。

-- app_meta：单行配置/状态键值对
CREATE TABLE IF NOT EXISTS app_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- settings：用户可改的运行时配置
CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

-- devices 扩展列
ALTER TABLE devices ADD COLUMN device_name    TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN paired_at      INTEGER;
ALTER TABLE devices ADD COLUMN is_paired      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE devices ADD COLUMN machine_anchor TEXT NOT NULL DEFAULT '';

-- device_credentials：每设备 MQTT 凭据
CREATE TABLE IF NOT EXISTS device_credentials (
    device_id            TEXT PRIMARY KEY,
    mqtt_username        TEXT NOT NULL,
    mqtt_credential_hash TEXT NOT NULL,         -- SHA-256 hex of password
    issued_at            INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    revoked_at           INTEGER,
    FOREIGN KEY (device_id) REFERENCES devices(device_id)
);

-- pairing_sessions：配对会话
CREATE TABLE IF NOT EXISTS pairing_sessions (
    token      TEXT PRIMARY KEY,
    device_id  TEXT,                             -- 配对成功后填
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    expires_at INTEGER NOT NULL,
    used_at    INTEGER,
    status     TEXT NOT NULL DEFAULT 'pending'   -- pending|success|expired|revoked
);
CREATE INDEX IF NOT EXISTS idx_pairing_status  ON pairing_sessions(status);
CREATE INDEX IF NOT EXISTS idx_pairing_expires ON pairing_sessions(expires_at);

-- api_keys：API key 持久化（哈希存储，明文只在生成时返回一次）
CREATE TABLE IF NOT EXISTS api_keys (
    key_id       TEXT PRIMARY KEY,               -- chk_<前 8 字符>
    key_hash     TEXT NOT NULL,                  -- SHA-256 hex of full key
    created_at   INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    revoked_at   INTEGER,
    last_used_at INTEGER,
    label        TEXT NOT NULL DEFAULT ''
);

-- security_events：安全审计日志
CREATE TABLE IF NOT EXISTS security_events (
    event_id     INTEGER PRIMARY KEY AUTOINCREMENT,
    type         TEXT NOT NULL,
    severity     TEXT NOT NULL DEFAULT 'info',
    payload_json TEXT,
    occurred_at  INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
CREATE INDEX IF NOT EXISTS idx_security_events_type ON security_events(type);
CREATE INDEX IF NOT EXISTS idx_security_events_time ON security_events(occurred_at);
