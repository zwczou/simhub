package utils

import (
	"strings"

	"github.com/fatih/structs"
	"github.com/spf13/cast"
	"github.com/uptrace/bun"
)

// QueryForm 用于将结构体参数映射为数据库查询的分页和排序条件
type QueryForm struct {
	v         any
	s         *structs.Struct
	nullFirst bool
	alias     string
}

// NewQueryForm 创建一个结构体查询条件解析器
func NewQueryForm(v any) *QueryForm {
	return &QueryForm{
		v: v,
		s: structs.New(v),
	}
}

// NullsFirst 设置排序时将空值 (NULL) 排在前面
func (f *QueryForm) NullsFirst() *QueryForm {
	f.nullFirst = true
	return f
}

// SetAlias 设置数据库表别名，防止在多表联查时排序字段冲突（如 alias.id）
func (f *QueryForm) SetAlias(alias string) *QueryForm {
	f.alias = alias
	return f
}

// Default 填充默认的分页和排序参数：Limit=25，按 ID 倒序排列
func (f *QueryForm) Default() *QueryForm {
	if limit, ok := f.s.FieldOk("Limit"); ok && limit.IsZero() {
		_ = limit.Set(int32(25))
	}
	if sort, ok := f.s.FieldOk("Sort"); ok && sort.IsZero() {
		_ = sort.Set("id")
	}
	if order, ok := f.s.FieldOk("Order"); ok && order.IsZero() {
		_ = order.Set("DESC")
	}
	return f
}

// Page 构建带有 offset 和 limit 的分页 SQL
func (f *QueryForm) Page(sql *bun.SelectQuery) *bun.SelectQuery {
	isAllField, ok1 := f.s.FieldOk("IsAll")
	if ok1 && cast.ToBool(isAllField.Value()) {
		return sql
	}
	pageField, ok2 := f.s.FieldOk("Page")
	offsetField, ok3 := f.s.FieldOk("Offset")
	limitField, ok4 := f.s.FieldOk("Limit")
	if ok4 {
		if ok3 && !offsetField.IsZero() {
			sql = sql.Offset(cast.ToInt(offsetField.Value()))
		}
		if ok2 && !pageField.IsZero() {
			page := cast.ToInt(pageField.Value())
			limit := cast.ToInt(limitField.Value())
			if page <= 0 {
				page = 1
			}
			sql = sql.Offset((page - 1) * limit)
		}
		sql = sql.Limit(cast.ToInt(limitField.Value()))
	}
	return sql
}

// Order 构建带有 ORDER BY 的排序 SQL
func (f *QueryForm) Order(sql *bun.SelectQuery) *bun.SelectQuery {
	sort := cast.ToString(f.s.Field("Sort").Value())
	order := strings.ToUpper(cast.ToString(f.s.Field("Order").Value()))
	suffix := " NULLS LAST"
	if f.nullFirst {
		suffix = ""
	}

	var key string
	if f.alias != "" {
		key = `"` + f.alias + `"."` + sort + `" ` + order + suffix
	} else {
		key = `"` + sort + `" ` + order + suffix
	}
	return sql.OrderExpr(key)
}

// Pageable 同时应用分页和排序条件到 SQL
func (f *QueryForm) Pageable(sql *bun.SelectQuery) *bun.SelectQuery {
	sql = f.Page(sql)
	sql = f.Order(sql)
	return sql
}

// DefaultPageable 填充默认参数后，应用分页和排序条件
func (f *QueryForm) DefaultPageable(sql *bun.SelectQuery) *bun.SelectQuery {
	return f.Default().Pageable(sql)
}

// BuildPageableQuery 提供一个闭包，快速应用默认分页和排序规则，适用于 bun 的 Apply 方法
func BuildPageableQuery(v any) func(*bun.SelectQuery) *bun.SelectQuery {
	return func(sql *bun.SelectQuery) *bun.SelectQuery {
		return NewQueryForm(v).DefaultPageable(sql)
	}
}

// Like 生成数据库 LIKE 模糊查询的格式字符串（前缀+后缀）
func Like(name string) string {
	return "%" + name + "%"
}

// PrefixLike 生成数据库 LIKE 前缀匹配查询的格式字符串
func PrefixLike(name string) string {
	return name + "%"
}
