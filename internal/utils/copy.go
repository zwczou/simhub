package utils

import (
	"context"
	"time"

	"github.com/jinzhu/copier"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

var copierOption copier.Option

func init() {
	copierOption.Converters = append(copierOption.Converters, copier.TypeConverter{
		SrcType: time.Duration(0),
		DstType: string(""),
		Fn: func(src any) (any, error) {
			if src == nil {
				return "", nil
			}
			dur := src.(time.Duration)
			return dur.String(), nil
		},
	})
	copierOption.Converters = append(copierOption.Converters, copier.TypeConverter{
		SrcType: string(""),
		DstType: time.Duration(0),
		Fn: func(src any) (any, error) {
			if src == nil {
				return time.Duration(0), nil
			}
			str := src.(string)
			if str == "" {
				return time.Duration(0), nil
			}
			dur, err := time.ParseDuration(str)
			if err != nil {
				return time.Duration(0), nil
			}
			return dur, nil
		},
	})
	copierOption.Converters = append(copierOption.Converters, copier.TypeConverter{
		SrcType: time.Time{},
		DstType: int64(0),
		Fn: func(src any) (any, error) {
			return src.(time.Time).Unix(), nil
		},
	})
	copierOption.Converters = append(copierOption.Converters, copier.TypeConverter{
		SrcType: &time.Time{},
		DstType: int64(0),
		Fn: func(src any) (any, error) {
			if src == nil {
				return int64(0), nil
			}
			tm := src.(*time.Time)
			if tm == nil {
				return int64(0), nil
			}
			return src.(*time.Time).Unix(), nil
		},
	})
	copierOption.Converters = append(copierOption.Converters, copier.TypeConverter{
		SrcType: int64(0),
		DstType: &time.Time{},
		Fn: func(src any) (any, error) {
			if src == nil {
				return nil, nil
			}
			if src.(int64) == 0 {
				return nil, nil
			}
			at := time.Unix(src.(int64), 0)
			return &at, nil
		},
	})
	copierOption.Converters = append(copierOption.Converters, copier.TypeConverter{
		SrcType: string(""),
		DstType: decimal.Decimal{},
		Fn: func(src any) (any, error) {
			if src.(string) == "" {
				return decimal.Zero, nil
			}
			return decimal.NewFromString(src.(string))
		},
	})
}

// 此处为啥封装一层
// 为了方便后面的替换，比如可以做到time.Time转换成int64，或者其他的转换
func Copy(ctx context.Context, dst, src any) {
	err := copier.CopyWithOption(dst, src, copierOption)
	if err != nil {
		log.Ctx(ctx).Error().Any("src", src).Any("dst", dst).Err(err).Send()
	}
}
