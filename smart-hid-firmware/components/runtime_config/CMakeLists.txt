# runtime_config — NVS 运行时配置（active/pending 双 namespace）
idf_component_register(
    SRCS "runtime_config.c"
    INCLUDE_DIRS "include"
    REQUIRES nvs_flash log
)
