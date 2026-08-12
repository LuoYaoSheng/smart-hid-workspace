-- Migration 0003: License 验签所需列（CL-3a）。
-- 设计源：smart-hid-cloud/docs/license-format.md
--
-- ControlHub 需要存储 Cloud 签发的完整 License（payload_json + signature），
-- 离线/在线激活后本地验签。CH-P1 的 licenses 表缺这些列。

-- 完整 License JSON（含 payload + signature），便于导入/刷新时复用
ALTER TABLE licenses ADD COLUMN payload_json    TEXT;
-- 验签字段（payload 内的副本，便于 SQL 查询）
ALTER TABLE licenses ADD COLUMN account_id      TEXT NOT NULL DEFAULT '';
ALTER TABLE licenses ADD COLUMN issued_at       INTEGER;
ALTER TABLE licenses ADD COLUMN license_version INTEGER NOT NULL DEFAULT 1;
