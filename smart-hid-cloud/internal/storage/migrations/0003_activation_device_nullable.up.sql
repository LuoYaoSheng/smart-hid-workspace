-- 0003: activation_codes.device_id 改为可空 + 移除 device FK（CL-6a）。
--
-- 原因：device_id NOT NULL + FK→devices 阻止了"通用激活码"（生成时不绑定具体设备，
-- 消费时由 ControlHub 提供设备并绑定）。且 Cloud 的 devices 表只是门户注册表，
-- ControlHub 才是配对的 source-of-truth——强制 Cloud device 预注册是不必要的耦合。
-- 真正的设备绑定由 License 签名强制（VerifyFull 校验 device_id）。
--
-- 保留 license_id / user_id FK（这两个是真实引用）。SQLite 改列需表重建。
CREATE TABLE activation_codes_new (
    code          TEXT PRIMARY KEY,
    license_id    TEXT NOT NULL,
    user_id       TEXT NOT NULL,
    device_id     TEXT,                             -- 现可空（NULL = 通用码）
    expires_at    INTEGER NOT NULL,
    used_at       INTEGER,
    created_at    INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    FOREIGN KEY (license_id) REFERENCES licenses(license_id),
    FOREIGN KEY (user_id) REFERENCES users(user_id)
);
INSERT INTO activation_codes_new(code, license_id, user_id, device_id, expires_at, used_at, created_at)
SELECT code, license_id, user_id, device_id, expires_at, used_at, created_at FROM activation_codes;
DROP TABLE activation_codes;
ALTER TABLE activation_codes_new RENAME TO activation_codes;
CREATE INDEX IF NOT EXISTS idx_activation_codes_user ON activation_codes(user_id);
