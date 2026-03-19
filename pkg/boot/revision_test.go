package boot

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestRead 测试 Read 能返回当前进程的基础构建信息。
func TestRead(t *testing.T) {
	cachedInfo.Store(nil)
	t.Cleanup(func() {
		cachedInfo.Store(nil)
	})

	info := Read()

	if info.GoVersion == "" {
		t.Fatal("expected GoVersion to be non-empty")
	}
	if info.GoVersion != runtime.Version() {
		t.Fatalf("expected GoVersion=%s, got %s", runtime.Version(), info.GoVersion)
	}
	t.Logf("GoVersion: %s", info.GoVersion)

	if info.Module == "" {
		t.Fatal("expected Module to be non-empty (run via go test)")
	}
	t.Logf("Module:    %s", info.Module)
	t.Logf("Version:   %s", info.Version)
}

// TestReadVCS 测试 Read 能读取 VCS 相关信息。
func TestReadVCS(t *testing.T) {
	info := Read()

	if info.VCSType != "" {
		t.Logf("VCS:       %s", info.VCSType)
		t.Logf("Revision:  %s", info.Revision)
		if info.Time != nil {
			t.Logf("Time:      %s", info.Time.Format(time.RFC3339))
		}
		t.Logf("Modified:  %v", info.Modified)
	} else {
		t.Log("VCS info not available (expected in non-git or cached builds)")
	}
}

// TestShort 测试 Short 的格式化输出。
func TestShort(t *testing.T) {
	tests := []struct {
		name     string
		info     Info
		expected string
	}{
		{
			name:     "version only",
			info:     Info{Version: "v1.0.0"},
			expected: "v1.0.0",
		},
		{
			name:     "with short revision",
			info:     Info{Version: "v1.0.0", Revision: "abc1234"},
			expected: "v1.0.0 (abc1234)",
		},
		{
			name:     "with long revision truncated",
			info:     Info{Version: "v1.0.0", Revision: "abc1234567890def"},
			expected: "v1.0.0 (abc1234)",
		},
		{
			name:     "dirty build",
			info:     Info{Version: "(devel)", Revision: "abc1234", Modified: true},
			expected: "(devel) (abc1234) [dirty]",
		},
		{
			name:     "devel no revision",
			info:     Info{Version: "(devel)"},
			expected: "(devel)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.Short()
			if got != tt.expected {
				t.Fatalf("Short() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestString 测试 String 的格式化输出。
func TestString(t *testing.T) {
	testTime, _ := time.Parse(time.RFC3339, "2026-03-13T10:00:00Z")
	info := Info{
		GoVersion: "go1.26.1",
		Module:    "github.com/game/game",
		Version:   "v1.2.3",
		VCSType:   "git",
		Revision:  "abc1234567890",
		Time:      &testTime,
		Modified:  true,
	}

	s := info.String()
	t.Logf("String output:\n%s", s)

	checks := []string{
		"github.com/game/game v1.2.3",
		"go1.26.1",
		"git",
		"abc1234567890",
		"2026-03-13T10:00:00Z",
		"modified:",
	}
	for _, c := range checks {
		if !strings.Contains(s, c) {
			t.Fatalf("String() missing %q", c)
		}
	}
}

// TestReadReturnsCopy 测试 Read 返回缓存副本而不是共享引用。
func TestReadReturnsCopy(t *testing.T) {
	testTime, _ := time.Parse(time.RFC3339, "2026-03-13T10:00:00Z")
	cachedInfo.Store(&Info{
		GoVersion: "go1.26.1",
		Module:    "github.com/iot/simhub",
		Version:   "v1.0.0",
		Time:      &testTime,
		Hostname:  "node-a",
	})
	t.Cleanup(func() {
		cachedInfo.Store(nil)
	})

	first := Read()
	first.Module = "changed-module"
	first.Hostname = "changed-host"
	*first.Time = first.Time.Add(time.Hour)

	second := Read()
	if second.Module != "github.com/iot/simhub" {
		t.Fatalf("expected cached module unchanged, got %q", second.Module)
	}
	if second.Hostname != "node-a" {
		t.Fatalf("expected cached hostname unchanged, got %q", second.Hostname)
	}
	if second.Time == nil || !second.Time.Equal(testTime) {
		t.Fatalf("expected cached time unchanged, got %v", second.Time)
	}
}

// TestReadIntegration 测试 Read 的集成输出。
func TestReadIntegration(t *testing.T) {
	info := Read()
	fmt.Println("=== Build Info ===")
	fmt.Println(info.String())
	fmt.Println("=== Short ===")
	fmt.Println(info.Short())
}
