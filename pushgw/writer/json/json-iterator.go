package json

import (
	"github.com/json-iterator/go"
	"math"
	"strconv"
	"unsafe"
)

// SafeFloat64 类型，用于处理 NaN 值
type SafeFloat64 float64

// MarshalJSON 方法自定义 JSON 编码，处理 NaN 为 null
func (f SafeFloat64) MarshalJSON() ([]byte, error) {
	if math.IsNaN(float64(f)) {
		return []byte("null"), nil
	}
	return []byte(strconv.FormatFloat(float64(f), 'f', -1, 64)), nil
}

// 自定义 JSON 配置：禁用掉 jsoniter 对 protobuf 内部字段的反射暴力解析
var customJson = jsoniter.Config{
	EscapeHTML:              true,
	SortMapKeys:             true,
	MarshalFloatWith6Digits: false,
	TagKey:                  "json",
	OnlyTaggedField:         true, // 只处理带 json tag 的字段，避免 XXX_ 字段被序列化
}.Froze()

// MarshalWithCustomFloat 用于自定义 float64 的 JSON 编码
func MarshalWithCustomFloat(items interface{}) ([]byte, error) {
	// 兼容处理 SafeFloat64
	jsoniter.RegisterTypeEncoderFunc("json.SafeFloat64",
		func(ptr unsafe.Pointer, stream *jsoniter.Stream) {
			val := *(*SafeFloat64)(ptr)
			if math.IsNaN(float64(val)) {
				stream.WriteNil()
			} else {
				stream.WriteFloat64(float64(val))
			}
		},
		func(ptr unsafe.Pointer) bool {
			return false
		},
	)
	return customJson.Marshal(items)
}
