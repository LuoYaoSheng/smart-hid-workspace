package sys

import (
	"net"
	"testing"
)

func TestAcquireSingleInstance_FirstOK(t *testing.T) {
	port := freePort(t)
	si, err := AcquireSingleInstance(port)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer si.Close()
}

func TestAcquireSingleInstance_SecondFails(t *testing.T) {
	port := freePort(t)
	first, err := AcquireSingleInstance(port)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer first.Close()

	second, err := AcquireSingleInstance(port)
	if err == nil {
		second.Close()
		t.Fatal("second acquire succeeded, want failure")
	}
}

// freePort 拿一个空闲端口，避免测试间端口冲突。
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}
