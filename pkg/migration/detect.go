package migration

// Op 表示 migration 执行的操作类型。
type Op string

const (
	OpNone Op = ""
	OpInit Op = "init"
	OpUp   Op = "up"
	OpDown Op = "down"
)

// DetectOp 从命令行参数中识别 migration 操作类型。
func DetectOp(args []string) Op {
	if len(args) == 0 {
		return OpNone
	}

	switch args[len(args)-1] {
	case string(OpInit):
		return OpInit
	case string(OpUp):
		return OpUp
	case string(OpDown):
		return OpDown
	default:
		return OpNone
	}
}
