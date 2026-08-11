# smart_hid_protocol — 协议契约（镜像 smart-ble/core/protocols/hid-command-schema.ts 事实源）
#
# 任何字段/常量变更必须先改 TS 事实源，再同步本组件。
idf_component_register(
    SRCS "smart_hid_protocol.c"
    INCLUDE_DIRS "."
    REQUIRES json
)
