package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const (
	appDirName     = "bootpane"
	configFileName = "workspace.json"
	// RepoConfigName 是放在仓库根目录、建议提交进 git 的配置文件名。
	RepoConfigName = ".bootpane.json"
)

// ErrProjectNotFound 表示注册表里没有这个项目。
var ErrProjectNotFound = errors.New("项目不存在")

// Store 管理全局注册表，并负责把它和仓库内的 .bootpane.json 合并成前端视图。
type Store struct {
	dir string

	mu sync.RWMutex
	ws Workspace
}

// NewStore 打开（或初始化）用户配置目录下的注册表。
func NewStore() (*Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("无法定位用户配置目录: %w", err)
	}
	dir := filepath.Join(base, appDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}
	s := &Store{dir: dir}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Dir 返回配置目录，用于界面上「打开配置位置」。
func (s *Store) Dir() string { return s.dir }

// FilePath 返回注册表文件的完整路径。
func (s *Store) FilePath() string { return filepath.Join(s.dir, configFileName) }

func (s *Store) load() error {
	data, err := os.ReadFile(s.FilePath())
	if errors.Is(err, os.ErrNotExist) {
		s.ws = Workspace{Version: 1, Projects: []Project{}}
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取 %s 失败: %w", configFileName, err)
	}
	var ws Workspace
	if err := json.Unmarshal(data, &ws); err != nil {
		return fmt.Errorf("%s 格式错误: %w", configFileName, err)
	}
	if ws.Projects == nil {
		ws.Projects = []Project{}
	}
	s.ws = ws
	return nil
}

// save 原子写入注册表：先写临时文件再重命名，避免中途崩溃留下半截 JSON。
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.ws, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.FilePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.FilePath())
}

// SeedIfEmpty 在注册表为空时，把给定的目录作为初始项目写入。
// 只在开发或首次运行时使用，避免用户打开应用看到一片空白。
func (s *Store) SeedIfEmpty(paths ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.ws.Projects) > 0 {
		return nil
	}
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		abs = filepath.Clean(abs)
		s.ws.Projects = append(s.ws.Projects, Project{
			ID:   makeID(abs),
			Name: filepath.Base(abs),
			Path: abs,
		})
	}
	if len(s.ws.Projects) == 0 {
		return nil
	}
	return s.save()
}

// Projects 返回所有项目的合并视图，按「置顶 + 名称」排序。
func (s *Store) Projects() []ResolvedProject {
	s.mu.RLock()
	list := make([]Project, len(s.ws.Projects))
	copy(list, s.ws.Projects)
	s.mu.RUnlock()

	out := make([]ResolvedProject, 0, len(list))
	for _, p := range list {
		out = append(out, s.Resolve(p))
	}
	sortProjects(out)
	return out
}

// Get 返回单个项目的合并视图。
func (s *Store) Get(id string) (ResolvedProject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.ws.Projects {
		if p.ID == id {
			return s.Resolve(p), nil
		}
	}
	return ResolvedProject{}, ErrProjectNotFound
}

// Add 把一个目录登记为项目。重复路径会返回已有项目而不是报错。
func (s *Store) Add(path string) (ResolvedProject, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ResolvedProject{}, fmt.Errorf("路径无效: %w", err)
	}
	abs = filepath.Clean(abs)
	fi, err := os.Stat(abs)
	if err != nil || !fi.IsDir() {
		return ResolvedProject{}, fmt.Errorf("目录不存在: %s", abs)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.ws.Projects {
		if strings.EqualFold(filepath.Clean(p.Path), abs) {
			return s.Resolve(p), nil
		}
	}
	p := Project{ID: makeID(abs), Name: filepath.Base(abs), Path: abs}
	s.ws.Projects = append(s.ws.Projects, p)
	if err := s.save(); err != nil {
		s.ws.Projects = s.ws.Projects[:len(s.ws.Projects)-1]
		return ResolvedProject{}, err
	}
	return s.Resolve(p), nil
}

// Remove 从注册表移除项目，不会碰磁盘上的仓库文件。
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.ws.Projects {
		if p.ID != id {
			continue
		}
		s.ws.Projects = append(s.ws.Projects[:i], s.ws.Projects[i+1:]...)
		return s.save()
	}
	return ErrProjectNotFound
}

// Resolve 把单个项目的两层配置合并成视图。调用方负责持有锁或拷贝。
func (s *Store) Resolve(p Project) ResolvedProject {
	rp := ResolvedProject{
		ID:        p.ID,
		Name:      p.Name,
		Path:      p.Path,
		Tags:      append([]string{}, p.Tags...),
		Pinned:    p.Pinned,
		Tasks:     []Task{},
		Pipelines: []Pipeline{},
		Issues:    []Issue{},
	}

	if fi, err := os.Stat(p.Path); err == nil && fi.IsDir() {
		rp.Exists = true
	} else {
		rp.Issues = append(rp.Issues, Issue{Level: "error", Message: "项目目录不存在或不可访问"})
	}

	var pf ProjectFile
	if rp.Exists {
		raw, err := os.ReadFile(filepath.Join(p.Path, RepoConfigName))
		switch {
		case err == nil:
			rp.HasRepoCfg = true
			if e := json.Unmarshal(raw, &pf); e != nil {
				rp.ConfigErr = fmt.Sprintf("%s 解析失败: %v", RepoConfigName, e)
				rp.Issues = append(rp.Issues, Issue{Level: "error", Message: rp.ConfigErr})
				pf = ProjectFile{}
			}
		case !errors.Is(err, os.ErrNotExist):
			rp.ConfigErr = fmt.Sprintf("读取 %s 失败: %v", RepoConfigName, err)
			rp.Issues = append(rp.Issues, Issue{Level: "warn", Message: rp.ConfigErr})
		}
	}

	if rp.Name == "" {
		rp.Name = pf.Project.Name
	}
	if rp.Name == "" {
		rp.Name = filepath.Base(p.Path)
	}
	if len(pf.Project.Tags) > 0 {
		rp.Tags = dedupe(append(rp.Tags, pf.Project.Tags...))
	}
	if rp.Tags == nil {
		rp.Tags = []string{}
	}

	rp.Tasks = mergeTasks(pf.Tasks, p.LocalTasks)
	if pf.Pipelines != nil {
		rp.Pipelines = pf.Pipelines
	}

	rp.Issues = append(rp.Issues, Validate(&rp)...)
	return rp
}

// mergeTasks 按 id 合并两层任务：仓库配置优先，个人层只补缺。
func mergeTasks(repo, local []Task) []Task {
	out := make([]Task, 0, len(repo)+len(local))
	seen := make(map[string]struct{}, len(repo))

	for _, t := range repo {
		t.Origin = OriginRepo
		if t.Label == "" {
			t.Label = t.ID
		}
		out = append(out, t)
		if t.ID != "" {
			seen[t.ID] = struct{}{}
		}
	}
	for _, t := range local {
		if _, dup := seen[t.ID]; dup {
			continue
		}
		t.Origin = OriginLocal
		if t.Label == "" {
			t.Label = t.ID
		}
		out = append(out, t)
	}
	return out
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// makeID 生成稳定且可读的项目标识：路径不变则 ID 不变，重命名目录也不影响。
func makeID(path string) string {
	norm := strings.ToLower(filepath.ToSlash(filepath.Clean(path)))
	sum := sha256.Sum256([]byte(norm))
	base := slugRe.ReplaceAllString(strings.ToLower(filepath.Base(path)), "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "project"
	}
	if len(base) > 24 {
		base = base[:24]
	}
	return base + "-" + hex.EncodeToString(sum[:])[:6]
}

func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func sortProjects(list []ResolvedProject) {
	// 置顶优先，然后按名称不区分大小写排序。数量级很小，插入排序足够。
	for i := 1; i < len(list); i++ {
		cur := list[i]
		j := i - 1
		for j >= 0 && lessProject(cur, list[j]) {
			list[j+1] = list[j]
			j--
		}
		list[j+1] = cur
	}
}

func lessProject(a, b ResolvedProject) bool {
	if a.Pinned != b.Pinned {
		return a.Pinned
	}
	return strings.ToLower(a.Name) < strings.ToLower(b.Name)
}
