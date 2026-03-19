package migration

import "testing"

// TestDetectOp 测试命令行参数到 migration 操作的识别逻辑。
func TestDetectOp(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
		want Op
	}{
		{
			name: "empty args",
			args: nil,
			want: OpNone,
		},
		{
			name: "detect init",
			args: []string{"simd", "init"},
			want: OpInit,
		},
		{
			name: "detect up",
			args: []string{"simd", "--config", "local.yml", "up"},
			want: OpUp,
		},
		{
			name: "detect down",
			args: []string{"simd", "down"},
			want: OpDown,
		},
		{
			name: "ignore unknown op",
			args: []string{"simd", "status"},
			want: OpNone,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := DetectOp(testCase.args); got != testCase.want {
				t.Fatalf("DetectOp() = %q, want %q", got, testCase.want)
			}
		})
	}
}
