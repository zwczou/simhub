package utils

import (
	"strings"
	"testing"
)

func TestMaskString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		length int
		want   string
	}{
		{"ascii short", "abc", 5, "abc"},
		{"ascii exact", "abcde", 5, "abcde"},
		{"ascii long", "abcdefg", 4, "ab...fg"},
		{"chinese short", "你好啊", 5, "你好啊"},
		{"chinese exact", "你好啊", 3, "你好啊"},
		{"chinese long", "一二三四五六七", 4, "一二...六七"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskString(tt.input, tt.length); got != tt.want {
				t.Errorf("MaskString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsEmail(t *testing.T) {
	valid := []string{"test@example.com", "a.b@c-d.com"}
	invalid := []string{"test", "test@", "@example.com"}

	for _, v := range valid {
		if !IsEmail(v) {
			t.Errorf("expected %s to be valid email", v)
		}
	}
	for _, v := range invalid {
		if IsEmail(v) {
			t.Errorf("expected %s to be invalid email", v)
		}
	}
}

func TestIsPhone(t *testing.T) {
	valid := []string{"13812345678", "02012345678"}
	invalid := []string{"123", "a13812345678", "138123456789011"}

	for _, v := range valid {
		if !IsPhone(v) {
			t.Errorf("expected %s to be valid phone", v)
		}
	}
	for _, v := range invalid {
		if IsPhone(v) {
			t.Errorf("expected %s to be invalid phone", v)
		}
	}
}

func TestNewTimeNo(t *testing.T) {
	no := NewTimeNo("PRE")
	if !strings.HasPrefix(no, "PRE") {
		t.Errorf("expected prefix PRE, got %s", no)
	}
	// PRE (3) + Date (12) + MS (3) + Rand (4) = 22
	if len(no) != 22 {
		t.Errorf("expected length 22, got %d (%s)", len(no), no)
	}
}

func TestFormatPhone(t *testing.T) {
	code, phone, err := FormatPhone(86, "13812345678")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if code != 86 || phone != "13812345678" {
		t.Errorf("got %d %s", code, phone)
	}
}

func TestObfuscate(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	res := Obfuscate(data)
	if len(res) != len(data) {
		t.Errorf("length mismatch")
	}
	res2 := Obfuscate(res)
	for i := range data {
		if data[i] != res2[i] {
			t.Errorf("obfuscate not symmetric")
		}
	}
}

func TestNewAESKeyIV(t *testing.T) {
	key, iv := NewAESKeyIV()
	if len(key) != 24 || len(iv) != 16 {
		t.Errorf("invalid lengths")
	}
}
