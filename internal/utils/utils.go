package utils

import (
	"fmt"
	mrand "math/rand"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/nyaruka/phonenumbers"
)

var (
	emailRule = regexp.MustCompile(`^[\w.-]+@([\w-]+)+(\.[\w-]+)+$`)
	phoneRule = regexp.MustCompile(`^0[1-9]{1}[\d]{4,12}$|^[1-9]{1}[\d]{5,13}$`)
)

// MaskString 截取并拼接字符串，将中间的部分用...代替（支持中文字符）
func MaskString(s string, length int) string {
	runes := []rune(s)
	if len(runes) <= length {
		return s
	}
	half := length / 2
	return string(runes[:half]) + "..." + string(runes[len(runes)-half:])
}

// IsEmail 判断是否是合法的邮箱格式
func IsEmail(account string) bool {
	return emailRule.MatchString(account)
}

// IsPhone 判断是否是合法的手机号格式
func IsPhone(account string) bool {
	return phoneRule.MatchString(account)
}

// NewTimeNo 根据前缀生成时间编号
func NewTimeNo(pre string) string {
	now := time.Now()
	// 时间部分 060102150405
	timeStr := now.Format("060102150405")
	// 毫秒部分 (直接格式化并拼接到一起，避免 strings.Replacer 损耗，%03d 处理前导零)
	msStr := fmt.Sprintf("%03d", now.Nanosecond()/1000000)
	// 随机 1000~9999 数值转字符串 (Go 1.20+ 自动初始化随机种子)
	randStr := strconv.Itoa(1000 + mrand.Intn(9000))
	return pre + timeStr + msStr + randStr
}

// FormatPhone 格式化手机号，防止用户在号码前加0
// 输入 areaCode(如82), phone(如012345678)
// 输出格式化后的 areaCode, phone, err
func FormatPhone(areaCode int32, phone string) (int32, string, error) {
	phoneStr := "+" + strconv.FormatInt(int64(areaCode), 10) + phone
	num, err := phonenumbers.Parse(phoneStr, "")
	if err != nil {
		return 0, "", err
	}
	return int32(num.GetCountryCode()), strconv.FormatUint(num.GetNationalNumber(), 10), nil
}

// Obfuscate 对字节数组进行混淆（首尾交换 + 位取反）
func Obfuscate(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	res := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		// 位取反并放在对称的位置
		res[len(data)-1-i] = ^data[i]
	}
	return res
}

// NewAESKeyIV 生成随机的 AES 密钥和向量 (AES-192: 24 bytes key, 16 bytes iv)
func NewAESKeyIV() (key []byte, iv []byte) {
	key = make([]byte, 24)
	iv = make([]byte, 16)
	copy(key, []byte(uuid.NewString()))
	copy(iv, []byte(uuid.NewString()))
	return
}
