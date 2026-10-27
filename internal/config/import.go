package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ImportResult 是一次导入的结果。
//
// 业务层面的失败（JSON 格式错、校验不通过）不通过 error 返回，而是放在
// OK/Error/Issues 里——Wails 的 (T, error) 约定下，error 非空时返回值会被丢弃，
// 那样前端就拿不到具体是哪个 task 出的问题了。
type ImportResult struct {
	OK       bool    `json:"ok"`
	Imported int     `json:"imported"`
	Error    string  `json:"error,omitempty"`
	Issues   []Issue `json:"issues"`
}

// ParseProjectFile 解析用户从大模型那里粘回来的内容。
//
// 模型经常会在 JSON 前后加解释文字，或者用 ```json 围栏包起来，
// 所以这里不直接 Unmarshal 原文，先做一次容错提取。
func ParseProjectFile(raw string) (ProjectFile, error) {
	pf, err := parseProjectFile(raw)
	if err != nil {
		return ProjectFile{}, err
	}
	if len(pf.Tasks) == 0 {
		return ProjectFile{}, errors.New("配置里没有任何 task")
	}
	return pf, nil
}

// ParseEditableConfig 解析界面编辑框里的内容。
//
// 和 ParseProjectFile 的唯一区别是允许任务列表为空：用户在编辑框里把启动项
// 全删了是正常操作（清空重来），不能拿「导入空配置没意义」的规则挡住他。
func ParseEditableConfig(raw string) (ProjectFile, error) {
	return parseProjectFile(raw)
}

func parseProjectFile(raw string) (ProjectFile, error) {
	body := extractJSONBlock(raw)
	if body == "" {
		return ProjectFile{}, errors.New("没找到 JSON 内容，请确认粘贴的是完整的配置")
	}
	var pf ProjectFile
	if err := json.Unmarshal([]byte(body), &pf); err != nil {
		return ProjectFile{}, fmt.Errorf("JSON 格式有误：%w", err)
	}
	return pf, nil
}

// extractJSONBlock 从一段可能夹带解释文字的文本里抠出 JSON 主体。
func extractJSONBlock(raw string) string {
	raw = strings.TrimSpace(raw)

	// 优先取代码围栏里的内容
	if i := strings.Index(raw, "```"); i >= 0 {
		rest := raw[i+3:]
		rest = strings.TrimPrefix(rest, "json")
		rest = strings.TrimPrefix(rest, "JSON")
		if j := strings.Index(rest, "```"); j >= 0 {
			raw = rest[:j]
		} else {
			raw = rest
		}
	}

	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return ""
	}
	return raw[start : end+1]
}
