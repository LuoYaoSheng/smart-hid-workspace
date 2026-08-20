# ble_provision — NimBLE Provisioning GATT 服务 + 分帧/JSON 协议层
#（ble_proto.c 为纯逻辑，host 单测直接编译；ble_provision.c 依赖 NimBLE）
idf_component_register(
    SRCS "ble_provision.c" "ble_proto.c"
    INCLUDE_DIRS "include"
    REQUIRES bt json provisioning device_identity runtime_config
    PRIV_REQUIRES log
)
