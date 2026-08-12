-- Migration 0001: Smart HID Cloud 初始 schema（CL-2a）。
-- 设计源：docs/07_SMART_HID_WEB_PRD_V1.0.md §8（数据模型）+
--         docs/10 验收清单 §E/F。

-- users：账号
CREATE TABLE IF NOT EXISTS users (
    user_id        TEXT PRIMARY KEY,           -- acc_<22hex>
    email          TEXT NOT NULL UNIQUE,
    password_hash  TEXT NOT NULL,              -- SHA-256(salt + password)，V1 简化（不引 bcrypt）
    password_salt  TEXT NOT NULL,
    display_name   TEXT NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

-- plans：套餐定义（admin 维护，V1 用 seed 数据）
CREATE TABLE IF NOT EXISTS plans (
    plan_id         TEXT PRIMARY KEY,           -- plan_<code>
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    price_cents     INTEGER NOT NULL,           -- 单位：分（避免浮点）
    currency        TEXT NOT NULL DEFAULT 'CNY',
    duration_days   INTEGER NOT NULL,           -- License 有效期天数
    features_json   TEXT NOT NULL DEFAULT '[]', -- JSON array of feature strings
    active          INTEGER NOT NULL DEFAULT 1,
    created_at      INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

-- orders：订单
CREATE TABLE IF NOT EXISTS orders (
    order_id       TEXT PRIMARY KEY,            -- ord_<22hex>
    user_id        TEXT NOT NULL,
    plan_id        TEXT NOT NULL,
    amount_cents   INTEGER NOT NULL,
    currency       TEXT NOT NULL DEFAULT 'CNY',
    status         TEXT NOT NULL DEFAULT 'pending', -- pending|paid|cancelled|refunded
    created_at     INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    paid_at        INTEGER,
    FOREIGN KEY (user_id) REFERENCES users(user_id),
    FOREIGN KEY (plan_id) REFERENCES plans(plan_id)
);
CREATE INDEX IF NOT EXISTS idx_orders_user ON orders(user_id);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);

-- payments：支付记录（V1 mock；后续接真实网关时填 provider_txn_id）
CREATE TABLE IF NOT EXISTS payments (
    payment_id      TEXT PRIMARY KEY,
    order_id        TEXT NOT NULL,
    provider        TEXT NOT NULL DEFAULT 'mock',
    provider_txn_id TEXT,
    amount_cents    INTEGER NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending', -- pending|success|failed
    paid_at         INTEGER,
    created_at      INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    FOREIGN KEY (order_id) REFERENCES orders(order_id)
);
CREATE INDEX IF NOT EXISTS idx_payments_order ON payments(order_id);

-- licenses：License（核心）
-- 状态机：UNUSED → ACTIVE → EXPIRED；外加 DISABLED / REVOKED
CREATE TABLE IF NOT EXISTS licenses (
    license_id     TEXT PRIMARY KEY,            -- lic_<22hex>
    user_id        TEXT NOT NULL,
    plan_id        TEXT NOT NULL,
    order_id       TEXT,
    status         TEXT NOT NULL DEFAULT 'UNUSED', -- UNUSED|ACTIVE|EXPIRED|DISABLED|REVOKED
    device_id      TEXT,                         -- 激活后绑定
    issued_at      INTEGER,                      -- 签发时间（激活后填）
    valid_from     INTEGER,
    expires_at     INTEGER,
    features_json  TEXT NOT NULL DEFAULT '[]',
    payload_json   TEXT,                         -- 完整 Payload JSON（激活后填，便于 ControlHub 重新验签）
    signature      TEXT,                         -- base64 Ed25519（激活后填）
    created_at     INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    activated_at   INTEGER,
    FOREIGN KEY (user_id) REFERENCES users(user_id),
    FOREIGN KEY (plan_id) REFERENCES plans(plan_id),
    FOREIGN KEY (order_id) REFERENCES orders(order_id)
);
CREATE INDEX IF NOT EXISTS idx_licenses_user ON licenses(user_id);
CREATE INDEX IF NOT EXISTS idx_licenses_status ON licenses(status);
CREATE INDEX IF NOT EXISTS idx_licenses_device ON licenses(device_id);

-- devices：用户绑定的设备（授权意义，非实时状态；docs/07 §7）
CREATE TABLE IF NOT EXISTS devices (
    device_id      TEXT PRIMARY KEY,            -- HID-XXXXXXXX（与 ESP32 device_id 一致）
    user_id        TEXT NOT NULL,
    display_name   TEXT NOT NULL DEFAULT '',
    paired_at      INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    FOREIGN KEY (user_id) REFERENCES users(user_id)
);
CREATE INDEX IF NOT EXISTS idx_devices_user ON devices(user_id);

-- activation_codes：离线激活码（用户在 ControlHub 生成 → 在 Web 输入 → 拿 license）
CREATE TABLE IF NOT EXISTS activation_codes (
    code          TEXT PRIMARY KEY,             -- 12 字符 base32
    license_id    TEXT NOT NULL,
    user_id       TEXT NOT NULL,
    device_id     TEXT NOT NULL,                -- 预绑设备
    expires_at    INTEGER NOT NULL,
    used_at       INTEGER,
    created_at    INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    FOREIGN KEY (license_id) REFERENCES licenses(license_id),
    FOREIGN KEY (user_id) REFERENCES users(user_id),
    FOREIGN KEY (device_id) REFERENCES devices(device_id)
);
CREATE INDEX IF NOT EXISTS idx_activation_codes_user ON activation_codes(user_id);
