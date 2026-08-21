/**
 * led_manager.h — 板载状态 LED
 *
 * 状态语义（tick 任务 50ms 轮询 wifi/mqtt/usb，不侵入现有组件）：
 *
 * | 状态               | 判定                        | WS2812   | 单色 LED      |
 * |--------------------|-----------------------------|----------|---------------|
 * | Wi-Fi 连接中        | Wi-Fi 未连（含上电初期）     | 黄色快闪  | 快闪          |
 * | 链路丢失            | 曾就绪后 Wi-Fi 断开          | 红色快闪  | 快闪          |
 * | MQTT 连接中         | Wi-Fi 通、MQTT 未通          | 青色慢闪  | 慢闪          |
 * | USB 未挂载          | 网络全通、USB 未枚举到宿主机 | 紫色双闪  | 双闪          |
 * | 就绪                | 全链路通                    | 绿色常亮  | 常亮          |
 * | 命令脉冲            | EXECUTED ack 时 led_manager_pulse() | 白色短闪 | 反相短闪 |
 *
 * 硬件类型 / GPIO / 亮度 / 脉冲时长见 Kconfig（Smart HID LED 菜单）。
 */
#pragma once

#ifdef __cplusplus
extern "C" {
#endif

/**
 * 初始化状态 LED 并启动 50ms 轮询任务。
 * 建议在装配早期调用（Wi-Fi/MQTT 未初始化时轮询安全，读到未连接）。
 *
 * @return 0 成功（含 LED 类型 = NONE 时的空实现）；非 0 失败
 */
int led_manager_init(void);

/**
 * 命令执行成功脉冲：覆盖当前状态显示 PULSE_MS 毫秒白闪（单色 LED 反相）。
 * 由 main.c 的 publisher 包装在 EXECUTED ack 时调用；线程安全。
 */
void led_manager_pulse(void);

#ifdef __cplusplus
}
#endif
