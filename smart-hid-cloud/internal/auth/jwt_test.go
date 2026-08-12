package auth

import (
	"testing"
	"time"
)

func TestSignVerify_HappyPath(t *testing.T) {
	secret := []byte("test-secret")
	c := Claims{UserID: "acc_abc"}
	tok, err := Sign(c, secret)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Verify(tok, secret)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.UserID != "acc_abc" {
		t.Errorf("user_id = %s", got.UserID)
	}
}

func TestVerify_WrongSecret(t *testing.T) {
	tok, _ := Sign(Claims{UserID: "x"}, []byte("secret1"))
	if _, err := Verify(tok, []byte("secret2")); err == nil {
		t.Errorf("verify passed with wrong secret")
	}
}

func TestVerify_Expired(t *testing.T) {
	c := Claims{UserID: "x", EXP: time.Now().Unix() - 1}
	tok, _ := SignWithTTL(c, []byte("s"), 0)
	// EXP 已设为过去，TTL=0 不会覆盖（SignWithTTL 只在 EXP=0 时设）
	if _, err := Verify(tok, []byte("s")); err == nil {
		t.Errorf("verify passed on expired token")
	}
}

func TestVerify_Malformed(t *testing.T) {
	cases := []string{"", "abc", "a.b", "a.b.c.d", "..."}
	for _, c := range cases {
		if _, err := Verify(c, []byte("s")); err == nil {
			t.Errorf("Verify(%q) passed, want error", c)
		}
	}
}

func TestSign_TTLApplied(t *testing.T) {
	before := time.Now().Unix()
	tok, _ := SignWithTTL(Claims{UserID: "x"}, []byte("s"), 1*time.Hour)
	after := time.Now().Unix()
	c, _ := Verify(tok, []byte("s"))
	if c.EXP < before+3500 || c.EXP > after+3700 {
		t.Errorf("EXP = %d, want ~%d+%d", c.EXP, before, 3600)
	}
}
