package boot

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRead(t *testing.T) {
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

func TestReadIntegration(t *testing.T) {
	info := Read()
	fmt.Println("=== Build Info ===")
	fmt.Println(info.String())
	fmt.Println("=== Short ===")
	fmt.Println(info.Short())
}
