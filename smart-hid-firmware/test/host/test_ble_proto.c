/**
 * test_ble_proto.c — BLE Provisioning 协议层 host 单测（分帧 + JSON）
 */
#include "test_framework.h"
#include "ble_proto.h"

#include <string.h>

static const char *CAND_JSON =
    "{\"v\":1,\"wifi_ssid\":\"home-net\",\"wifi_password\":\"pass1234\","
    "\"hub_host\":\"192.168.1.8\",\"hub_port\":17892,"
    "\"token\":\"0123456789abcdef0123456789abcdef\"}";

/* 单帧专用短载荷（≤ CHUNK_MAX） */
static const char *SHORT_JSON =
    "{\"v\":1,\"wifi_ssid\":\"net\",\"wifi_password\":\"p\","
    "\"hub_host\":\"192.168.1.8\",\"token\":\"t32\"}";

/* 单帧（短 payload 一块装下） */
void test_ble_frame_single_chunk(void) {
    ble_frame_assembler_t a;
    ble_frame_reset(&a);
    uint8_t frame[3 + 128];
    size_t json_len = strlen(SHORT_JSON);
    CHECK(json_len <= BLE_PROTO_CHUNK_MAX, "test json fits one chunk");
    frame[0] = 0; frame[1] = 1; frame[2] = (uint8_t)json_len;
    memcpy(frame + 3, SHORT_JSON, json_len);
    CHECK(ble_frame_feed(&a, frame, (uint16_t)(3 + json_len)) == BLE_PROTO_COMPLETE, "single chunk completes");
    CHECK(a.len == json_len, "assembled length");
    CHECK(memcmp(a.buf, SHORT_JSON, json_len) == 0, "payload intact");
}

/* 多帧顺序重组（spec §25：不假设单包装下） */
void test_ble_frame_multi_chunk_ordered(void) {
    ble_frame_assembler_t a;
    ble_frame_reset(&a);
    const char *p = CAND_JSON;
    size_t total = strlen(p);
    /* 故意用 7 字节小块（模拟 MTU 23 场景） */
    size_t chunk = 7;
    int nchunks = (int)((total + chunk - 1) / chunk);
    CHECK(nchunks > 1, "multiple chunks");
    for (int i = 0; i < nchunks; i++) {
        size_t off = (size_t)i * chunk;
        size_t len = (off + chunk <= total) ? chunk : total - off;
        uint8_t frame[3 + 32];
        frame[0] = (uint8_t)i;
        frame[1] = (uint8_t)nchunks;
        frame[2] = (uint8_t)len;
        memcpy(frame + 3, p + off, len);
        ble_proto_result_t r = ble_frame_feed(&a, frame, (uint16_t)(3 + len));
        if (i == nchunks - 1) CHECK(r == BLE_PROTO_COMPLETE, "last chunk completes");
        else CHECK(r == BLE_PROTO_NEED_MORE, "intermediate chunk needs more");
    }
    CHECK(a.len == total, "assembled full length");
    CHECK(memcmp(a.buf, CAND_JSON, total) == 0, "reassembled payload identical");
}

/* 乱序/跳号拒绝；seq=0 重启新传输 */
void test_ble_frame_out_of_order_rejected(void) {
    ble_frame_assembler_t a;
    ble_frame_reset(&a);
    uint8_t f[3 + 4] = {0, 3, 4, 'a', 'b', 'c', 'd'};
    CHECK(ble_frame_feed(&a, f, 7) == BLE_PROTO_NEED_MORE, "chunk 0 ok");
    uint8_t f2[3 + 4] = {2, 3, 4, 'e', 'f', 'g', 'h'}; /* 跳过 seq1 */
    CHECK(ble_frame_feed(&a, f2, 7) == BLE_PROTO_ERR_SEQ, "skip rejected");
    /* 客户端从 seq=0 重发 */
    CHECK(ble_frame_feed(&a, f, 7) == BLE_PROTO_NEED_MORE, "restart from seq0 ok");
}

/* 非法帧：len 与实际不符 / 超 chunk 上限 / total=0 */
void test_ble_frame_malformed(void) {
    ble_frame_assembler_t a;
    ble_frame_reset(&a);
    uint8_t short_frame[2] = {0, 1};
    CHECK(ble_frame_feed(&a, short_frame, 2) == BLE_PROTO_ERR_FRAME, "under 3 bytes rejected");
    uint8_t bad_total[3] = {0, 0, 0};
    CHECK(ble_frame_feed(&a, bad_total, 3) == BLE_PROTO_ERR_FRAME, "total=0 rejected");
    uint8_t bad_len[3 + 4] = {0, 1, 9, 'x', 'y', 'z', 'w'}; /* len=9 但只带 4 */
    CHECK(ble_frame_feed(&a, bad_len, 7) == BLE_PROTO_ERR_FRAME, "len mismatch rejected");
}

/* candidate 解析：默认端口 / 缺字段拒绝 / 版本拒绝 */
void test_ble_parse_candidate(void) {
    runtime_candidate_t c;
    CHECK(ble_proto_parse_candidate(CAND_JSON, strlen(CAND_JSON), &c) == 0, "valid parses");
    CHECK(strcmp(c.wifi_ssid, "home-net") == 0, "ssid");
    CHECK(strcmp(c.hub_host, "192.168.1.8") == 0, "hub host");
    CHECK(c.hub_port == 17892, "hub port");
    CHECK(strcmp(c.token, "0123456789abcdef0123456789abcdef") == 0, "token");

    const char *no_port = "{\"v\":1,\"wifi_ssid\":\"s\",\"wifi_password\":\"p\","
                          "\"hub_host\":\"h\",\"token\":\"t\"}";
    CHECK(ble_proto_parse_candidate(no_port, strlen(no_port), &c) == 0, "port optional");
    CHECK(c.hub_port == 17892, "default port 17892");

    const char *no_token = "{\"v\":1,\"wifi_ssid\":\"s\",\"wifi_password\":\"p\",\"hub_host\":\"h\"}";
    CHECK(ble_proto_parse_candidate(no_token, strlen(no_token), &c) != 0, "missing token rejected");

    const char *v2 = "{\"v\":2,\"wifi_ssid\":\"s\",\"wifi_password\":\"p\","
                     "\"hub_host\":\"h\",\"token\":\"t\"}";
    CHECK(ble_proto_parse_candidate(v2, strlen(v2), &c) != 0, "future version rejected");

    const char *bad = "not json";
    CHECK(ble_proto_parse_candidate(bad, strlen(bad), &c) != 0, "garbage rejected");
}

/* info / status JSON 形状 + 密钥不入状态 */
void test_ble_build_info_status(void) {
    char buf[256];
    CHECK(ble_proto_build_info(buf, sizeof(buf), "HID-ABCD1234", "1.1.0", PROV_PROVISIONING, false) == 0, "info builds");
    CHECK(strstr(buf, "\"device_id\":\"HID-ABCD1234\"") != NULL, "info device_id");
    CHECK(strstr(buf, "\"state\":\"provisioning\"") != NULL, "info state");
    CHECK(strstr(buf, "\"provisioned\":false") != NULL, "info provisioned flag");

    CHECK(ble_proto_build_status(buf, sizeof(buf), PROV_CONNECTING_WIFI, "connecting_wifi", PROV_ERR_NONE) == 0, "status builds");
    CHECK(strstr(buf, "\"error\":null") != NULL, "no error → null");
    CHECK(strstr(buf, "\"state\":\"connecting_wifi\"") != NULL, "status state");

    CHECK(ble_proto_build_status(buf, sizeof(buf), PROV_PROVISIONING, "wifi_failed", PROV_ERR_WIFI_FAILED) == 0, "err status builds");
    CHECK(strstr(buf, "\"error\":\"wifi_failed\"") != NULL, "stable error code string");
}
