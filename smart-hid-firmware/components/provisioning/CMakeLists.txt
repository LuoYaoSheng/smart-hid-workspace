# provisioning — 配网状态机（纯编排，host 可测；依赖注入 wifi/pairing/mqtt 适配器）
idf_component_register(
    SRCS "provisioning.c"
    INCLUDE_DIRS "include"
    REQUIRES runtime_config
    PRIV_REQUIRES log
)
