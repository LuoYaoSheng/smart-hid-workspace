-- Migration 0004: command 幂等指纹（M1-G2）
-- commands 新列 fingerprint：request_id 重放/并发时判断"是否同一命令"的
-- canonical 指纹（sha256(device_id|type|action|canonical payload)）。
-- 兼容性：旧行 fingerprint=''（视为未知内容）——重放旧行时不判冲突，
-- 直接按既有状态回放，避免升级后历史命令重放变成 409。

ALTER TABLE commands ADD COLUMN fingerprint TEXT NOT NULL DEFAULT '';
