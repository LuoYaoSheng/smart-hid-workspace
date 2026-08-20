# Smart HID Firmware — ESP32-S3
# ESP-IDF >= v5.0；版本事实源 = 仓库根 VERSION（M1-G4 唯一版本源）
cmake_minimum_required(VERSION 3.16)

# PROJECT_VER 注入：读取根 VERSION（去掉可能的前缀 v 与空白）。
# esp_app_desc（镜像头）自动携带该版本，运行时 esp_app_get_description()->version
# 可读——device_identity_get_firmware() 即来源于此，不再硬编码。
file(READ "${CMAKE_CURRENT_LIST_DIR}/../VERSION" _ver_raw)
string(STRIP "${_ver_raw}" _ver)
string(REGEX REPLACE "^v" "" PROJECT_VER "${_ver}")
message(STATUS "Smart HID firmware version: ${PROJECT_VER}")

include($ENV{IDF_PATH}/tools/cmake/project.cmake)

project(smart-hid-firmware)
