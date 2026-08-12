# Smart HID ControlHub Makefile
# 常用：make test | make run | make windows-installer | make clean

GO ?= go

.PHONY: all build run test test-short vet fmt clean windows-installer mock-device

all: build

# 本机构建（开发验证，macOS 直接跑）
build:
	$(GO) build -o bin/controlhub ./cmd/controlhub

# 本机启动 ControlHub（headless 模式；要 tray 加 ARGS=-tray）
run: build
	./bin/controlhub $(ARGS)

# mock-device（开发联调用，本机连本机 broker）
mock-device:
	$(GO) build -o bin/mock-device ./cmd/mock-device

# 全套测试（含 D3 idle timeout ~12s 测试）
test:
	$(GO) test ./... -count=1

# 短测试（跳过 D3 等慢测）
test-short:
	$(GO) test ./... -count=1 -short

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

# Windows 安装包（交叉编译 + NSIS；详见 packaging/build-windows.sh）
windows-installer:
	packaging/build-windows.sh

clean:
	rm -rf bin packaging/ControlHub.exe packaging/ControlHub.exe.manifest packaging/ControlHub_Setup.exe
