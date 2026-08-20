/**
 * nvs_stub.c — 进程内 NVS 实现（host 测试）
 *
 * 语义忠实点：写入先进 stage，nvs_commit 成功才落 base；commit 失败或
 * 不 commit 即丢弃（对应真实 NVS 掉电丢未提交写）。读取只看 base。
 */
#include "nvs.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define MAX_NS 8
#define MAX_KEYS 64

typedef struct {
    char key[16];
    bool is_str;
    char sval[160];
    uint32_t ival;
} kv_t;

typedef struct {
    char name[16];
    kv_t kv[MAX_KEYS];      /* base：已提交 */
    kv_t stage[MAX_KEYS];   /* 本次事务未提交写 */
    int nstage;
    bool stage_erase_all;
} ns_t;

static ns_t g_ns[MAX_NS];
static bool g_fail_commit = false;
static char g_fail_ns[16] = {0};

static ns_t *find_ns(const char *name, bool create) {
    for (int i = 0; i < MAX_NS; i++) {
        if (strcmp(g_ns[i].name, name) == 0) return &g_ns[i];
    }
    if (!create) return NULL;
    for (int i = 0; i < MAX_NS; i++) {
        if (g_ns[i].name[0] == '\0') {
            snprintf(g_ns[i].name, sizeof(g_ns[i].name), "%s", name);
            return &g_ns[i];
        }
    }
    return NULL;
}

/* 手写两份查找避免 C 里没有方法 */
static kv_t *find_base(ns_t *ns, const char *key, bool create) {
    int n = (int)(sizeof(ns->kv) / sizeof(kv_t));
    (void)n;
    for (int i = 0; i < MAX_KEYS; i++) {
        if (ns->kv[i].key[0] == '\0') break;
        if (strcmp(ns->kv[i].key, key) == 0) return &ns->kv[i];
    }
    if (!create) return NULL;
    for (int i = 0; i < MAX_KEYS; i++) {
        if (ns->kv[i].key[0] == '\0') {
            snprintf(ns->kv[i].key, sizeof(ns->kv[i].key), "%s", key);
            return &ns->kv[i];
        }
    }
    return NULL;
}

static kv_t *find_stage(ns_t *ns, const char *key) {
    for (int i = 0; i < ns->nstage; i++) {
        if (strcmp(ns->stage[i].key, key) == 0) return &ns->stage[i];
    }
    if (ns->nstage >= MAX_KEYS) return NULL;
    kv_t *kv = &ns->stage[ns->nstage++];
    snprintf(kv->key, sizeof(kv->key), "%s", key);
    return kv;
}

static ns_t *handle_ns(nvs_handle_t h) {
    if (h == 0 || h > MAX_NS) return NULL;
    ns_t *ns = &g_ns[h - 1];
    return ns->name[0] ? ns : NULL;
}

esp_err_t nvs_open(const char *ns, int mode, nvs_handle_t *out) {
    (void)mode;
    if (ns == NULL || out == NULL) return ESP_FAIL;
    ns_t *n = find_ns(ns, true);
    if (n == NULL) return ESP_FAIL;
    *out = (nvs_handle_t)((n - g_ns) + 1);
    return ESP_OK;
}

esp_err_t nvs_get_str(nvs_handle_t h, const char *key, char *out, size_t *len) {
    ns_t *n = handle_ns(h);
    if (n == NULL) return ESP_FAIL;
    kv_t *kv = find_base(n, key, false);
    if (kv == NULL || !kv->is_str) return ESP_ERR_NVS_NOT_FOUND;
    size_t need = strlen(kv->sval) + 1;
    if (out == NULL) {
        if (len) *len = need;
        return ESP_OK;
    }
    if (len == NULL || *len < need) return ESP_FAIL;
    memcpy(out, kv->sval, need);
    *len = need;
    return ESP_OK;
}

static esp_err_t get_int(nvs_handle_t h, const char *key, uint32_t *out) {
    ns_t *n = handle_ns(h);
    if (n == NULL) return ESP_FAIL;
    kv_t *kv = find_base(n, key, false);
    if (kv == NULL || kv->is_str) return ESP_ERR_NVS_NOT_FOUND;
    if (out) *out = kv->ival;
    return ESP_OK;
}

esp_err_t nvs_get_u8(nvs_handle_t h, const char *key, uint8_t *out) {
    uint32_t v;
    esp_err_t e = get_int(h, key, &v);
    if (e == ESP_OK && out) *out = (uint8_t)v;
    return e;
}
esp_err_t nvs_get_u16(nvs_handle_t h, const char *key, uint16_t *out) {
    uint32_t v;
    esp_err_t e = get_int(h, key, &v);
    if (e == ESP_OK && out) *out = (uint16_t)v;
    return e;
}
esp_err_t nvs_get_u32(nvs_handle_t h, const char *key, uint32_t *out) {
    return get_int(h, key, out);
}

static esp_err_t stage_write(nvs_handle_t h, const char *key, bool is_str,
                             const char *sval, uint32_t ival) {
    ns_t *n = handle_ns(h);
    if (n == NULL) return ESP_FAIL;
    kv_t *kv = find_stage(n, key);
    if (kv == NULL) return ESP_FAIL;
    kv->is_str = is_str;
    if (is_str) snprintf(kv->sval, sizeof(kv->sval), "%s", sval);
    else kv->ival = ival;
    return ESP_OK;
}

esp_err_t nvs_set_str(nvs_handle_t h, const char *key, const char *val) {
    return stage_write(h, key, true, val, 0);
}
esp_err_t nvs_set_u8(nvs_handle_t h, const char *key, uint8_t v) {
    return stage_write(h, key, false, NULL, v);
}
esp_err_t nvs_set_u16(nvs_handle_t h, const char *key, uint16_t v) {
    return stage_write(h, key, false, NULL, v);
}
esp_err_t nvs_set_u32(nvs_handle_t h, const char *key, uint32_t v) {
    return stage_write(h, key, false, NULL, v);
}

esp_err_t nvs_erase_all(nvs_handle_t h) {
    ns_t *n = handle_ns(h);
    if (n == NULL) return ESP_FAIL;
    n->stage_erase_all = true;
    n->nstage = 0;
    return ESP_OK;
}

esp_err_t nvs_commit(nvs_handle_t h) {
    ns_t *n = handle_ns(h);
    if (n == NULL) return ESP_FAIL;

    bool fail = (g_fail_ns[0] != '\0' && strcmp(n->name, g_fail_ns) == 0);
    if (g_fail_commit) {
        g_fail_commit = false;
        fail = true;
    }
    if (fail) {
        n->nstage = 0;         /* 丢弃未提交写 */
        n->stage_erase_all = false;
        return ESP_FAIL;
    }

    if (n->stage_erase_all) {
        memset(n->kv, 0, sizeof(n->kv));
        n->stage_erase_all = false;
    }
    for (int i = 0; i < n->nstage; i++) {
        kv_t *dst = find_base(n, n->stage[i].key, true);
        if (dst == NULL) return ESP_FAIL;
        *dst = n->stage[i];
    }
    n->nstage = 0;
    return ESP_OK;
}

esp_err_t nvs_close(nvs_handle_t h) {
    (void)h;
    return ESP_OK;
}

void nvs_stub_reset(void) {
    memset(g_ns, 0, sizeof(g_ns));
    g_fail_commit = false;
    g_fail_ns[0] = '\0';
}

void nvs_stub_fail_commit(bool fail) { g_fail_commit = fail; }
void nvs_stub_fail_ns(const char *ns) { snprintf(g_fail_ns, sizeof(g_fail_ns), "%s", ns); }
void nvs_stub_reset_fail_ns(void) { g_fail_ns[0] = '\0'; }

int nvs_stub_raw_set_u8(const char *ns, const char *key, uint8_t v) {
    ns_t *n = find_ns(ns, true);
    if (n == NULL) return -1;
    kv_t *kv = find_base(n, key, true);
    if (kv == NULL) return -1;
    kv->is_str = false;
    kv->ival = v;
    return 0;
}
