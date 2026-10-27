// Package discovery 分析仓库结构，自动推导出可能的启动方式。
//
// 设计成「规则引擎先行、LLM 补充、校验兜底」三层：
//  1. 规则层是确定性的，覆盖 package.json / monorepo / go / docker-compose 等常见结构；
//  2. LLM 层只负责补规则漏掉的组合，且输出必须过同一个校验器；
//  3. 校验层拦下「命令里引用的 script 根本不存在」这类幻觉。
//
// 生成结果永远先出 diff 预览，用户确认后才落盘，不直接生效。
package discovery

import (
	"context"
	"errors"

	"bootpane/internal/config"
)

// ErrNotImplemented 表示扫描器尚未接入（M4 实现）。
var ErrNotImplemented = errors.New("仓库扫描尚未实现")

// Candidate 是一条候选启动配置，带来源和置信度。
//
// Confidence 和 Evidence 不是装饰：UI 上要把「来自 package.json 的 scripts.dev」
// 这类依据展示给用户，让他判断该不该采纳，而不是盲信一个黑盒结果。
type Candidate struct {
	Task       config.Task `json:"task"`
	Confidence float64     `json:"confidence"`
	Evidence   string      `json:"evidence"`
	Source     string      `json:"source"`
}

// Detector 是一类仓库特征的识别器，每种生态一个实现。
type Detector interface {
	Name() string
	Detect(ctx context.Context, root string) ([]Candidate, error)
}

// Scan 对仓库根目录跑一遍所有识别器。
func Scan(ctx context.Context, root string) ([]Candidate, error) {
	return nil, ErrNotImplemented
}
