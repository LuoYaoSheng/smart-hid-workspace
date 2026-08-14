-- Migration 0004: 需求与反馈（FB-1）。
-- 匿名公开提交 + admin 分诊（new → planned/shipped/rejected）。
-- 设计源：docs/feedback.md。

-- feedback：用户提交的需求 / Bug / 其他反馈
CREATE TABLE IF NOT EXISTS feedback (
    feedback_id  TEXT PRIMARY KEY,            -- fb_<22hex>
    user_id      TEXT,                        -- 预留（V1 恒空：匿名提交；不设 FK，与账号解耦）
    category     TEXT NOT NULL,               -- feature|bug|other
    title        TEXT NOT NULL,               -- ≤80 字符（服务端校验）
    body         TEXT NOT NULL,               -- ≤2000 字符
    contact      TEXT,                        -- 可选联系方式（邮箱等，≤120）
    client_ip    TEXT,                        -- 提交来源 IP（反垃圾审计用，不公开）
    user_agent   TEXT,                        -- 提交 UA（截断 250）
    status       TEXT NOT NULL DEFAULT 'new', -- new|planned|shipped|rejected
    admin_note   TEXT,                        -- admin 备注（planned/shipped 时对外可见）
    created_at   INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    updated_at   INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
CREATE INDEX IF NOT EXISTS idx_feedback_status ON feedback(status);
CREATE INDEX IF NOT EXISTS idx_feedback_created ON feedback(created_at);
