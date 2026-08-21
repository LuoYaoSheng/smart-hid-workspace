/**
 * ble_provision.c — NimBLE Provisioning GATT 服务实现（ESP-IDF v5.4 NimBLE API）
 *
 * 结构（spec §32）：GATT 回调只做 transport → parser → on_candidate；
 * 全部配网逻辑在 provisioning 组件（纯逻辑，host 可测）。
 *
 * 安全：bonding + LE Secure Connections + Just Works（无 IO 能力，非 MITM
 * 抗性——如实声明，见 protocols/ble/PROVISIONING_V1.md §7）；Provision Input
 * 特征带 ENC 标志，未加密链路写入被协议栈拒绝。
 */
#include "ble_provision.h"

#include <stdio.h>
#include <string.h>

#include "esp_log.h"
#include "host/ble_gap.h"
#include "host/ble_gatt.h"
#include "host/ble_hs.h"
#include "host/util/util.h"
#include "nimble/nimble_port.h"
#include "nimble/nimble_port_freertos.h"

#include "device_identity.h"

static const char *TAG = "ble_provision";

/* ble_store_config_init 由 store/config 组件提供（无公共头，按官方示例前向声明） */
void ble_store_config_init(void);

static ble_provision_candidate_cb s_on_candidate = NULL;
static ble_frame_assembler_t s_asm;
static uint16_t s_conn_handle = BLE_HS_CONN_HANDLE_NONE;
static bool s_advertising = false;
static bool s_adv_wanted = false;   /* 广播意图：host 同步前 set_advertising(true) 先挂起，on_sync 后兑现 */

/* 最新 info / status JSON（read 值 + notify 载荷） */
static char s_info_json[192];
static char s_status_json[160];
static uint16_t s_val_handles[3]; /* 0=info 1=write 2=status（注册后填） */

static int gap_event(struct ble_gap_event *event, void *arg);

/* ----------------------------------------------------------------
 * UUID（canonical，见 protocols/ble/PROVISIONING_V1.md §2）
 * ---------------------------------------------------------------- */
static ble_uuid128_t g_svc_uuid =
    BLE_UUID128_INIT(0x04, 0x1c, 0x8a, 0x5e, 0x0b, 0x6f, 0x2a, 0x9d,
                     0x8f, 0x4c, 0x3b, 0xe7, 0x01, 0x10, 0x1d, 0x9f);
static ble_uuid128_t g_chr_info =
    BLE_UUID128_INIT(0x04, 0x1c, 0x8a, 0x5e, 0x0b, 0x6f, 0x2a, 0x9d,
                     0x8f, 0x4c, 0x3b, 0xe7, 0x02, 0x10, 0x1d, 0x9f);
static ble_uuid128_t g_chr_write =
    BLE_UUID128_INIT(0x04, 0x1c, 0x8a, 0x5e, 0x0b, 0x6f, 0x2a, 0x9d,
                     0x8f, 0x4c, 0x3b, 0xe7, 0x03, 0x10, 0x1d, 0x9f);
static ble_uuid128_t g_chr_status =
    BLE_UUID128_INIT(0x04, 0x1c, 0x8a, 0x5e, 0x0b, 0x6f, 0x2a, 0x9d,
                     0x8f, 0x4c, 0x3b, 0xe7, 0x04, 0x10, 0x1d, 0x9f);

/* ----------------------------------------------------------------
 * 特征访问回调
 * ---------------------------------------------------------------- */
static int chr_info_access(uint16_t conn, uint16_t attr,
                           struct ble_gatt_access_ctxt *ctxt, void *arg) {
    (void)conn; (void)attr; (void)arg;
    if (ctxt->op == BLE_GATT_ACCESS_OP_READ_CHR) {
        ble_proto_build_info(s_info_json, sizeof(s_info_json),
                             device_identity_get_device_id(),
                             device_identity_get_firmware(),
                             provisioning_state(), provisioning_is_provisioned());
        int rc = os_mbuf_append(ctxt->om, s_info_json, strlen(s_info_json));
        return rc == 0 ? 0 : BLE_ATT_ERR_INSUFFICIENT_RES;
    }
    return BLE_ATT_ERR_UNLIKELY;
}

static int chr_write_access(uint16_t conn, uint16_t attr,
                            struct ble_gatt_access_ctxt *ctxt, void *arg) {
    (void)conn; (void)attr; (void)arg;
    if (ctxt->op != BLE_GATT_ACCESS_OP_WRITE_CHR) return BLE_ATT_ERR_UNLIKELY;

    uint16_t len = OS_MBUF_PKTLEN(ctxt->om);
    if (len > 3 + BLE_PROTO_CHUNK_MAX) return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN;

    uint8_t frame[3 + BLE_PROTO_CHUNK_MAX];
    uint16_t copied = 0;
    if (ble_hs_mbuf_to_flat(ctxt->om, frame, sizeof(frame), &copied) != 0) {
        return BLE_ATT_ERR_UNLIKELY;
    }
    if (copied != len) return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN;

    ble_proto_result_t r = ble_frame_feed(&s_asm, frame, len);
    switch (r) {
        case BLE_PROTO_NEED_MORE:
            return 0;
        case BLE_PROTO_COMPLETE: {
            runtime_candidate_t cand;
            if (ble_proto_parse_candidate((const char *)s_asm.buf, s_asm.len, &cand) != 0) {
                ESP_LOGW(TAG, "candidate JSON invalid (len=%u)", s_asm.len);
                ble_provision_publish(provisioning_state(), "invalid_payload",
                                      PROV_ERR_INVALID_PAYLOAD);
                return 0; /* 帧合法，业务错误经 status char 反馈 */
            }
            ESP_LOGI(TAG, "candidate: ssid=%s hub=%s:%u token=**redacted**",
                     cand.wifi_ssid, cand.hub_host, (unsigned)cand.hub_port);
            if (s_on_candidate) s_on_candidate(&cand);
            return 0;
        }
        case BLE_PROTO_ERR_SEQ:
            return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN; /* 客户端从 seq=0 重发 */
        default:
            ble_frame_reset(&s_asm);
            return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN;
    }
}

static int chr_status_access(uint16_t conn, uint16_t attr,
                             struct ble_gatt_access_ctxt *ctxt, void *arg) {
    (void)conn; (void)attr; (void)arg;
    if (ctxt->op == BLE_GATT_ACCESS_OP_READ_CHR) {
        int rc = os_mbuf_append(ctxt->om, s_status_json, strlen(s_status_json));
        return rc == 0 ? 0 : BLE_ATT_ERR_INSUFFICIENT_RES;
    }
    return BLE_ATT_ERR_UNLIKELY;
}

/* ----------------------------------------------------------------
 * GATT 注册
 * ---------------------------------------------------------------- */
static const struct ble_gatt_chr_def g_chrs[] = {
    {
        .uuid = &g_chr_info.u,
        .access_cb = chr_info_access,
        .flags = BLE_GATT_CHR_F_READ | BLE_GATT_CHR_F_NOTIFY,
        .val_handle = &s_val_handles[0],
    },
    {
        .uuid = &g_chr_write.u,
        .access_cb = chr_write_access,
        /* 加密链路必须先建立（Just Works 配对），否则协议栈拒绝写入 */
        .flags = BLE_GATT_CHR_F_WRITE_ENC,
        .val_handle = &s_val_handles[1],
    },
    {
        .uuid = &g_chr_status.u,
        .access_cb = chr_status_access,
        .flags = BLE_GATT_CHR_F_READ | BLE_GATT_CHR_F_NOTIFY,
        .val_handle = &s_val_handles[2],
    },
    { 0 },
};

static const struct ble_gatt_svc_def g_svcs[] = {
    {
        .type = BLE_GATT_SVC_TYPE_PRIMARY,
        .uuid = &g_svc_uuid.u,
        .characteristics = g_chrs,
    },
    { 0 },
};

/* ----------------------------------------------------------------
 * GAP：广播 / 连接 / 安全
 * ---------------------------------------------------------------- */
static void start_advertising(void) {
    if (s_conn_handle != BLE_HS_CONN_HANDLE_NONE) return; /* 已连接不广播 */

    struct ble_hs_adv_fields fields = {0};
    fields.flags = BLE_HS_ADV_F_DISC_GEN | BLE_HS_ADV_F_BREDR_UNSUP;
    fields.uuids128 = &g_svc_uuid;
    fields.num_uuids128 = 1;
    fields.uuids128_is_complete = 1;

    int rc = ble_gap_adv_set_fields(&fields);
    if (rc != 0) {
        ESP_LOGE(TAG, "adv fields failed: %d", rc);
        return;
    }

    /* scan response 放设备名（广播包放不下 128bit UUID + 名字） */
    char name[24];
    snprintf(name, sizeof(name), "SHID-%s", device_identity_get_device_id() + 4);
    struct ble_hs_adv_fields rsp = {0};
    rsp.name = (const uint8_t *)name;
    rsp.name_len = (uint8_t)strlen(name);
    rsp.name_is_complete = 1;
    rc = ble_gap_adv_rsp_set_fields(&rsp);
    if (rc != 0) {
        ESP_LOGE(TAG, "adv rsp failed: %d", rc);
        return;
    }

    struct ble_gap_adv_params params = {0};
    params.conn_mode = BLE_GAP_CONN_MODE_UND;
    params.disc_mode = BLE_GAP_DISC_MODE_GEN;
    rc = ble_gap_adv_start(BLE_OWN_ADDR_PUBLIC, NULL, BLE_HS_FOREVER,
                           &params, gap_event, NULL);
    if (rc != 0 && rc != BLE_HS_EALREADY) {
        ESP_LOGE(TAG, "adv start failed: %d", rc);
        return;
    }
    s_advertising = true;
    ESP_LOGI(TAG, "provisioning advertising as %s", name);
}

static int gap_event(struct ble_gap_event *event, void *arg) {
    (void)arg;
    switch (event->type) {
        case BLE_GAP_EVENT_CONNECT:
            if (event->connect.status == 0) {
                s_conn_handle = event->connect.conn_handle;
                s_advertising = false;
                ESP_LOGI(TAG, "BLE connected handle=%u", s_conn_handle);
                /* 发起配对加密（参数由 ble_hs_cfg.sm_* 决定：bond+SC+Just Works） */
                ble_gap_security_initiate(s_conn_handle);
            } else {
                s_conn_handle = BLE_HS_CONN_HANDLE_NONE;
                start_advertising();
            }
            return 0;

        case BLE_GAP_EVENT_DISCONNECT:
            s_conn_handle = BLE_HS_CONN_HANDLE_NONE;
            ESP_LOGI(TAG, "BLE disconnected (reason=%d)", event->disconnect.reason);
            ble_frame_reset(&s_asm);
            start_advertising();
            return 0;

        case BLE_GAP_EVENT_ADV_COMPLETE:
            start_advertising();
            return 0;

        case BLE_GAP_EVENT_ENC_CHANGE:
            ESP_LOGI(TAG, "encryption %s (status=%d)",
                     event->enc_change.status == 0 ? "on" : "FAILED",
                     event->enc_change.status);
            return 0;

        case BLE_GAP_EVENT_SUBSCRIBE:
            ESP_LOGI(TAG, "subscribe: attr=%u notify=%d",
                     event->subscribe.attr_handle, event->subscribe.cur_notify);
            return 0;

        case BLE_GAP_EVENT_MTU:
            ESP_LOGI(TAG, "MTU update: %u", event->mtu.value);
            return 0;

        default:
            return 0;
    }
}

static void on_sync(void) {
    ble_hs_util_ensure_addr(0);
    /* 启动早期 set_advertising(true) 会因 host 未同步失败（ENOTSYNCED=22），
     * 意图挂在 s_adv_wanted，同步完成后在此兑现（真机 2026-08-21 修复） */
    if (s_adv_wanted && !s_advertising) start_advertising();
}

static void on_reset(int reason) {
    ESP_LOGE(TAG, "nimble reset: %d", reason);
    /* host reset 后控制器广播已死但 s_advertising 残留 true，会挡住
     * on_sync 的意图兑现（s_adv_wanted && !s_advertising 恒假）。
     * 清残留，让下次 on_sync 按 s_adv_wanted 恢复广播。 */
    s_advertising = false;
}

static void host_task(void *param) {
    nimble_port_run(); /* 阻塞直到 nimble_port_stop */
    nimble_port_freertos_deinit();
    (void)param;
}

/* ----------------------------------------------------------------
 * 公开 API
 * ---------------------------------------------------------------- */
int ble_provision_init(const char *device_name, ble_provision_candidate_cb on_candidate) {
    s_on_candidate = on_candidate;
    ble_frame_reset(&s_asm);

    /* nimble_port_init 完成 controller + host 栈初始化（IDF 5.x） */
    esp_err_t err = nimble_port_init();
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "nimble_port_init: %s", esp_err_to_name(err));
        return -1;
    }

    ble_hs_cfg.sync_cb = on_sync;
    ble_hs_cfg.reset_cb = on_reset;
    /* Just Works + bonding + LE Secure Connections（无 IO 能力，非 MITM 抗性，
     * 如实记录于协议文档 §7） */
    ble_hs_cfg.sm_io_cap = BLE_HS_IO_NO_INPUT_OUTPUT;
    ble_hs_cfg.sm_bonding = 1;
    ble_hs_cfg.sm_sc = 1;
    ble_hs_cfg.sm_mitm = 0;

    int rc = ble_gatts_count_cfg(g_svcs);
    if (rc != 0) return rc;
    rc = ble_gatts_add_svcs(g_svcs);
    if (rc != 0) return rc;

    /* bond 持久化（NVS，重连免重配） */
    ble_store_config_init();

    nimble_port_freertos_init(host_task);
    ESP_LOGI(TAG, "ble provision service initialized (%s)", device_name);
    return 0;
}

void ble_provision_set_advertising(bool on) {
    if (on) {
        s_adv_wanted = true;
        /* host 已同步才尝试；否则等 on_sync（ble_hs_synced 查询同步态） */
        if (!s_advertising && ble_hs_synced()) start_advertising();
    } else {
        s_adv_wanted = false;
        if (s_advertising) {
            ble_gap_adv_stop();
            s_advertising = false;
            ESP_LOGI(TAG, "provisioning advertising stopped");
        }
    }
}

void ble_provision_publish(prov_state_t st, const char *step, prov_error_t err) {
    ble_proto_build_status(s_status_json, sizeof(s_status_json), st, step, err);
    ble_proto_build_info(s_info_json, sizeof(s_info_json),
                         device_identity_get_device_id(),
                         device_identity_get_firmware(),
                         provisioning_state(), provisioning_is_provisioned());
    if (s_conn_handle != BLE_HS_CONN_HANDLE_NONE) {
        struct os_mbuf *om = ble_hs_mbuf_from_flat(s_status_json, strlen(s_status_json));
        if (om != NULL) {
            ble_gattc_notify_custom(s_conn_handle, s_val_handles[2], om);
        }
        om = ble_hs_mbuf_from_flat(s_info_json, strlen(s_info_json));
        if (om != NULL) {
            ble_gattc_notify_custom(s_conn_handle, s_val_handles[0], om);
        }
    }
}
