package utils

import (
	"strings"

	"github.com/spf13/cast"
)

// Replacer 基于键值对提供简单的字符串模板替换功能
type Replacer struct {
	data map[string]string
}

// NewReplacer 根据可变参数（键名和键值交替）构造 Replacer。如果参数数量为奇数则会 panic
func NewReplacer(v ...any) *Replacer {
	if len(v)%2 != 0 {
		panic("invalid arguments")
	}
	data := make(map[string]string)
	for i := 0; (i + 1) < len(v); i += 2 {
		data[cast.ToString(v[i])] = cast.ToString(v[i+1])
	}
	return &Replacer{
		data: data,
	}
}

// NewReplacerFromMap 根据现有的数据字典构造 Replacer
func NewReplacerFromMap(data map[string]string) *Replacer {
	return &Replacer{
		data: data,
	}
}

// With 向模板字典中添加或者更新键值对，命名更符合 Builder 模式的链式调用语义
func (r *Replacer) With(k, v any) *Replacer {
	r.data[cast.ToString(k)] = cast.ToString(v)
	return r
}

// Replace 执行替换：它会将字符串内形如 {key} 的模板内容替换为对应的值
func (r *Replacer) Replace(s string) string {
	vs := make([]string, 0, len(r.data)*2)
	for k, v := range r.data {
		vs = append(vs, "{"+k+"}", v)
	}
	return strings.NewReplacer(vs...).Replace(s)
}

// FormatTemplate 提供一个全局快捷方法，无需手动实例化即可执行一次性替换
// 用法：utils.FormatTemplate("hello {name}", "name", "world")
func FormatTemplate(template string, v ...any) string {
	return NewReplacer(v...).Replace(template)
}
