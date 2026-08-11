# Smart HID MQTT Command Protocol & ControlHub HTTP API v1.0

## 1. Topic

```text
smart-hid/v1/devices/{device_id}/command
smart-hid/v1/devices/{device_id}/ack
smart-hid/v1/devices/{device_id}/status
smart-hid/v1/devices/{device_id}/event
```

## 2. QoS / Retain

```text
command  QoS1  retain=false
ack      QoS1  retain=false
status   QoS1  retain=true
event    QoS0/1 retain=false
```

严禁 retained command。

## 3. Command Envelope

```json
{
  "protocol": "1.0",
  "request_id": "cmd_01JABCDEFG",
  "device_id": "HID-A82F94C1",
  "target_boot_id": "B-7DB289",
  "type": "keyboard",
  "action": "tap",
  "ttl_ms": 3000,
  "payload": {
    "key": "ENTER"
  }
}
```

## 4. ACK

```json
{
  "protocol": "1.0",
  "request_id": "cmd_01JABCDEFG",
  "device_id": "HID-A82F94C1",
  "boot_id": "B-7DB289",
  "status": "executed",
  "code": 0,
  "execution_ms": 6
}
```

状态：
- received
- executing
- executed
- rejected
- expired
- duplicate

## 5. 去重

QoS1 是至少一次交付，因此 ESP32 必须按 `request_id` 去重。

建议：
- RAM 缓存最近 256 条 request_id
- duplicate 不重复执行 HID
- boot_id 改变后旧 Session Command 拒绝

## 6. Keyboard

### tap
```json
{
  "type": "keyboard",
  "action": "tap",
  "payload": {
    "key": "ENTER",
    "hold_ms": 40
  }
}
```

### hotkey
```json
{
  "type": "keyboard",
  "action": "hotkey",
  "payload": {
    "keys": ["CTRL", "SHIFT", "S"],
    "hold_ms": 50
  }
}
```

### key_down
必须带 lease_ms。

### key_up
提前结束 lease。

## 7. Mouse

- move(dx, dy)
- click(button, count)
- button_down(button, lease_ms)
- button_up(button)
- wheel(delta)

V1 仅 relative mouse，不做 absolute mouse。

## 8. System

```text
release_all
```

用于释放所有 Keyboard Key / Modifier / Mouse Button。

## 9. HTTP API

Base：

```text
http://127.0.0.1:{port}/api/v1
```

V1：

```text
GET  /health
GET  /devices
GET  /devices/{device_id}
POST /devices/{device_id}/commands
GET  /commands/{request_id}
GET  /license
GET  /usage
```

## 10. Auth

除 health 外，控制 API 使用 Bearer API Key。

默认只监听：
`127.0.0.1`

LAN API 必须用户显式开启。

## 11. Trial / License Gate

```text
HTTP Request
→ Auth
→ Validate
→ Device
→ Entitlement
→ Rate Limit
→ MQTT
```

Entitlement 必须在 Publish MQTT 前完成。

## 12. V1 不支持

- Shell
- Script
- Process launch
- File transfer
- Clipboard read
- Screen capture
- Key logging
- Arbitrary code execution
