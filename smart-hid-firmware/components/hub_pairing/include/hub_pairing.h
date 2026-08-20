/**
 * hub_pairing.h — ControlHub 设备侧配对 HTTP 客户端（M1-G3）
 *
 * POST http://<hub_host>:<hub_port>/api/v1/pairing/device
 * 请求：{token, device_id, boot_id, firmware, hardware}
 * 响应：{mqtt_host, mqtt_port, mqtt_username, mqtt_credential, device_id}
 *
 * 设备信任响应里的 mqtt_host（ControlHub advertised 地址）——设备不自己猜
 * broker 地址（spec M1-G3 §29）。
 */
#ifndef HUB_PAIRING_H
#define HUB_PAIRING_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    char    mqtt_host[64];
    uint16_t mqtt_port;
    char    mqtt_username[24];
    char    mqtt_password[65];
} hub_pairing_creds_t;

/**
 * 执行配对。返回：
 *   0        成功（creds_out 填充）
 *   -404     token 不存在（pairing_invalid）
 *   -410     token 过期/吊销（pairing_expired）
 *   -409     token 已消费（pairing_used）
 *   -503     ControlHub endpoint 解析失败（advertise unresolved；token 未消费，可重试）
 *   -1       网络不可达 / 响应非法（controlhub_unreachable）
 */
int hub_pairing_perform(const char *host, uint16_t port, const char *token,
                        const char *device_id, const char *boot_id,
                        const char *firmware, const char *hardware,
                        hub_pairing_creds_t *creds_out);

#ifdef __cplusplus
}
#endif
#endif /* HUB_PAIRING_H */
