/**
 * ble_proto.h — BLE Provisioning 传输协议（分帧 + JSON）纯逻辑层（M1-G3）
 *
 * 契约事实源：protocols/ble/PROVISIONING_V1.md。
 * 本文件不含 NimBLE 头 —— host 单测直接编译（spec §32）。
 */
#ifndef BLE_PROTO_H
#define BLE_PROTO_H

#include <stddef.h>
#include <stdint.h>

#include "provisioning.h"
#include "runtime_config.h"

#ifdef __cplusplus
extern "C" {
#endif

/* 分帧：[seq u8][total u8][len u8][payload ≤ len]
 * - chunk payload ≤ BLE_PROTO_CHUNK_MAX（客户端按协商 MTU 切块，MTU 23 时每块 ≤17B 也合法）
 * - 组装上限 BLE_PROTO_ASSEMBLE_MAX（candidate JSON 足够）
 */
#define BLE_PROTO_CHUNK_MAX    128
#define BLE_PROTO_ASSEMBLE_MAX 1024

typedef enum {
    BLE_PROTO_NEED_MORE = 0,   /* 继续等下一块 */
    BLE_PROTO_COMPLETE = 1,    /* 全部收齐（此时 assembler->buf 可解析） */
    BLE_PROTO_ERR_FRAME = -1,  /* 帧格式非法（len 超限等） */
    BLE_PROTO_ERR_SEQ   = -2,  /* 乱序/跳号 → 需客户端从 seq=0 重发 */
    BLE_PROTO_ERR_OVERFLOW = -3,
} ble_proto_result_t;

typedef struct {
    uint8_t  buf[BLE_PROTO_ASSEMBLE_MAX];
    uint16_t len;
    uint8_t  total;    /* 0 = 未开始 */
    uint8_t  next_seq; /* 期望收到的下一个 seq */
} ble_frame_assembler_t;

void ble_frame_reset(ble_frame_assembler_t *a);
ble_proto_result_t ble_frame_feed(ble_frame_assembler_t *a,
                                  const uint8_t *data, uint16_t data_len);

/* candidate JSON（PROVISIONING_V1 §Provision Input）→ runtime_candidate_t。
 * 返回 0 成功；-1 字段缺失/非法/版本不支持。hub_port 缺省 = 17892。 */
int ble_proto_parse_candidate(const char *json, size_t json_len,
                              runtime_candidate_t *out);

/* Device Info char JSON */
int ble_proto_build_info(char *buf, size_t buflen, const char *device_id,
                         const char *firmware, prov_state_t st, bool provisioned);

/* Status char JSON：{"state":"...","step":"...","error":null|"code"} */
int ble_proto_build_status(char *buf, size_t buflen, prov_state_t st,
                           const char *step, prov_error_t err);

#ifdef __cplusplus
}
#endif
#endif /* BLE_PROTO_H */
