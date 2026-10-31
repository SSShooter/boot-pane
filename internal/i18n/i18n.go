// Package i18n 提供后端文案的多语言支持。
//
// 后端生成的文字（校验问题、启动失败原因、生成的 prompt）最终都会显示在
// 界面上，所以它们和前端共用同一套语言设置：前端把当前语言同步过来，
// 这里按语言取文案。当前语言缺某条时回落到英文，英文也没有就直接返回
// key——界面上出现一个点分标识，比静默显示空字符串好排查。
package i18n

import (
	"fmt"
	"strings"
	"sync"
)

// Lang 是受支持的语言标识。用不带地区的短码，前端和后端只在这四个值上对齐。
type Lang string

const (
	ZH Lang = "zh"
	EN Lang = "en"
	PT Lang = "pt"
	RU Lang = "ru"
)

// Supported 列出全部语言，顺序即设置页下拉框里的展示顺序。
var Supported = []Lang{ZH, EN, PT, RU}

// Default 是没选过语言、系统语言也认不出来时的兜底语言。
const Default = EN

// 默认英文：前端首次启动会把系统语言同步过来，所以中文环境照样显示中文。
var (
	mu      sync.RWMutex
	current = Default
)

// Normalize 把 "zh-CN"、"pt_BR" 这类系统语言标记折成受支持的语言标识。
//
// 不认识时返回空串，由调用方决定回落到哪个语言——这里不替调用方猜。
func Normalize(raw string) Lang {
	v := strings.ToLower(strings.TrimSpace(raw))
	v = strings.ReplaceAll(v, "_", "-")
	if base, _, ok := strings.Cut(v, "-"); ok {
		v = base
	}
	switch Lang(v) {
	case ZH, EN, PT, RU:
		return Lang(v)
	}
	return ""
}

// Set 切换当前语言，返回是否被支持。空串表示不改变当前语言。
func Set(raw string) bool {
	lang := Normalize(raw)
	if lang == "" {
		return false
	}
	mu.Lock()
	current = lang
	mu.Unlock()
	return true
}

// Current 返回当前语言。
func Current() Lang {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// T 取一条文案并填入参数。
//
// 参数按 fmt 的动词填充，所以同一条文案在各语言里必须用同样的动词顺序。
func T(key string, args ...any) string {
	msg := lookup(Current(), key)
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}

func lookup(lang Lang, key string) string {
	if msg, ok := catalog[lang][key]; ok {
		return msg
	}
	if msg, ok := catalog[EN][key]; ok {
		return msg
	}
	return key
}
