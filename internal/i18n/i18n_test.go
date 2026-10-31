package i18n

import (
	"regexp"
	"strings"
	"testing"
)

// TestCatalogsShareKeys 保证每种语言都有同一套 key。
//
// 缺一条不会导致编译失败，只会在界面上悄悄回落到英文，所以必须在测试里卡住：
// 新增一条文案时最容易漏掉的就是「只加了中文」。
func TestCatalogsShareKeys(t *testing.T) {
	for _, lang := range Supported {
		msgs := catalog[lang]
		for key := range enMessages {
			if _, ok := msgs[key]; !ok {
				t.Errorf("%s 缺少文案 %q", lang, key)
			}
		}
		for key := range msgs {
			if _, ok := enMessages[key]; !ok {
				t.Errorf("%s 多出文案 %q（英文里没有）", lang, key)
			}
		}
	}
}

// TestCatalogVerbOrder 保证同一条文案在各语言里的占位符顺序一致。
//
// 译文把 %s 和 %d 调换位置不会报错，只会把端口号填进任务名里，
// 这类错误只靠人工核对很难发现。
func TestCatalogVerbOrder(t *testing.T) {
	for _, lang := range Supported {
		for key, want := range enMessages {
			got, ok := catalog[lang][key]
			if !ok {
				continue // 缺 key 由 TestCatalogsShareKeys 报
			}
			if a, b := verbClasses(want), verbClasses(got); a != b {
				t.Errorf("%s 的 %q 占位符不匹配：英文 %q，本语言 %q", lang, key, a, b)
			}
		}
	}
}

var verbRe = regexp.MustCompile(`%[-+ #0-9.]*([a-zA-Z])`)

// verbClasses 把格式化动词折成「数值 / 文本」两类序列。
//
// 不比较具体动词：%v、%s 在译文里互换是正常的，但 %d 换成 %s 会把数字
// 当字符串塞进句子，必须拦住。
func verbClasses(msg string) string {
	var out strings.Builder
	for _, m := range verbRe.FindAllStringSubmatch(msg, -1) {
		switch m[1] {
		case "d", "f", "x", "X":
			out.WriteString("N")
		default:
			out.WriteString("S")
		}
	}
	return out.String()
}

// TestDefaultIsEnglish 钉住兜底语言。
//
// 改它会让所有「没存过偏好」的用户换了界面语言，所以必须是显式决定，
// 不能顺手跟着别的改动一起变。
func TestDefaultIsEnglish(t *testing.T) {
	if Default != EN {
		t.Errorf("默认语言应为 en，实际 %q", Default)
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]Lang{
		"zh":      ZH,
		"zh-CN":   ZH,
		"ZH_hans": ZH,
		"en-US":   EN,
		"en":      EN,
		"pt-BR":   PT,
		"pt_BR":   PT,
		"RU-ru":   RU,
		"":        "",
		"  ":      "",
		"fr":      "",
		"de-DE":   "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestSetAndLookup(t *testing.T) {
	orig := Current()
	t.Cleanup(func() { Set(string(orig)) })

	if !Set("pt-br") {
		t.Fatal("pt-br 应被接受")
	}
	if Current() != PT {
		t.Fatalf("当前语言应为 pt，实际 %q", Current())
	}
	if got := T("svc.taskNotFound", "web"); !strings.Contains(got, "web") {
		t.Errorf("参数应填入文案，实际 %q", got)
	}
	if got := T("no.such.key"); got != "no.such.key" {
		t.Errorf("未知 key 应原样返回，实际 %q", got)
	}
	// 不认识的语言不该悄悄改掉当前语言。
	if Set("fr") {
		t.Error("不支持的语言应返回 false")
	}
	if Current() != PT {
		t.Errorf("Set 失败后语言不该变化，实际 %q", Current())
	}
}
