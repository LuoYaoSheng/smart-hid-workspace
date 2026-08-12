-- Migration 0002: users 加 role 列（CL-5a Admin 鉴权）。
-- role 取值：'user'（默认，普通用户）| 'admin'（管理员，可访问 /api/v1/admin/*）。
-- 已有用户默认 'user'，向后兼容。第一个 admin 由 config.admin_email 在启动时 promote。

ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user';
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
