package boot

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"time"
)

var cachedInfo atomic.Pointer[Info]

// Info 包含构建时的版本和 VCS 信息
type Info struct {
	GoVersion  string     // Go 编译器版本，如 "go1.26.1"
	Module     string     // 模块路径
	Version    string     // 模块版本（通常 "(devel)" 表示本地构建）
	VCSType    string     // 版本控制系统类型，如 "git"
	Revision   string     // VCS commit hash
	Time       *time.Time // VCS commit 时间
	Modified   bool       // 工作目录是否有未提交的修改
	IntranetIP string     // 内网 IP 地址
	Hostname   string     // 主机名
}

// Read 返回进程级构建与运行时信息快照。
func Read() Info {
	if info := cachedInfo.Load(); info != nil {
		return cloneInfo(*info)
	}

	info := collectInfo()
	cachedInfo.CompareAndSwap(nil, &info)

	if cached := cachedInfo.Load(); cached != nil {
		return cloneInfo(*cached)
	}
	return cloneInfo(info)
}

// collectInfo 采集当前进程的构建信息和运行时主机信息。
func collectInfo() Info {
	info := Info{
		GoVersion: runtime.Version(),
	}

	bi, ok := debug.ReadBuildInfo()
	if ok {
		info.Module = bi.Main.Path
		info.Version = bi.Main.Version

		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs":
				info.VCSType = s.Value
			case "vcs.revision":
				info.Revision = s.Value
			case "vcs.time":
				if s.Value != "" {
					if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
						info.Time = &t
					}
				}
			case "vcs.modified":
				info.Modified = s.Value == "true"
			}
		}
	}

	info.IntranetIP = getIntranetIP()
	info.Hostname, _ = os.Hostname()
	return info
}

// cloneInfo 返回 Info 的副本，避免调用方修改缓存内容。
func cloneInfo(info Info) Info {
	out := info
	if info.Time != nil {
		t := *info.Time
		out.Time = &t
	}
	return out
}

// getIntranetIP 获取本机的 IPv4 地址。优先返回内网（私有）地址，如果没有，则返回找到的第一个非回环公网地址。
func getIntranetIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	var fallbackIP string
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ip := ipnet.IP.To4(); ip != nil {
				// 优先返回内网（私有）地址，例如 192.168.x.x, 10.x.x.x 等
				if ip.IsPrivate() {
					return ip.String()
				}
				// 暂存第一个非回环公网地址作为兜底
				if fallbackIP == "" {
					fallbackIP = ip.String()
				}
			}
		}
	}
	return fallbackIP
}

// Short 返回简短的版本字符串，如 "v1.0.0 (abc1234)"
func (i Info) Short() string {
	s := i.Version
	if i.Revision != "" {
		rev := i.Revision
		if len(rev) > 7 {
			rev = rev[:7]
		}
		s += " (" + rev + ")"
	}
	if i.Modified {
		s += " [dirty]"
	}
	return s
}

// String 返回完整的版本信息
func (i Info) String() string {
	s := i.Module + " " + i.Version

	appendField := func(key, value string) {
		s += fmt.Sprintf("\n  %-12s %s", key+":", value)
	}

	appendField("go", i.GoVersion)
	if i.VCSType != "" {
		appendField("vcs", i.VCSType)
	}
	if i.Revision != "" {
		appendField("revision", i.Revision)
	}
	if i.Time != nil {
		appendField("time", i.Time.Format(time.RFC3339))
	}
	if i.Modified {
		appendField("modified", "true")
	}
	if i.IntranetIP != "" {
		appendField("intranet_ip", i.IntranetIP)
	}
	if i.Hostname != "" {
		appendField("hostname", i.Hostname)
	}
	return s
}
