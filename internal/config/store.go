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
	return NewStoreAt(filepath.Join(base, appDirName))
}

// NewStoreAt 在指定目录打开注册表。
//
// 主要给测试用：集成测试不能往用户真实的 %APPDATA% 里写东西。
// 将来要支持自定义配置目录时也是从这里接。
func NewStoreAt(dir string) (*Store, error) {
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
	stars := append([]StarRef(nil), s.ws.Stars...)
	s.mu.RUnlock()

	out := make([]ResolvedProject, 0, len(list))
	for _, p := range list {
		out = append(out, s.resolve(p, stars))
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
			return s.resolve(p, s.ws.Stars), nil
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
			return s.resolve(p, s.ws.Stars), nil
		}
	}
	p := Project{ID: makeID(abs), Name: filepath.Base(abs), Path: abs}
	s.ws.Projects = append(s.ws.Projects, p)
	if err := s.save(); err != nil {
		s.ws.Projects = s.ws.Projects[:len(s.ws.Projects)-1]
		return ResolvedProject{}, err
	}
	return s.resolve(p, s.ws.Stars), nil
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
		// 项目没了，它的星标也留着没意义——重新添加会生成同一个 ID，
		// 旧星标会「诈尸」冒出来，跟用户刚移除的意图相反。
		s.ws.Stars = dropStarsOf(s.ws.Stars, id)
		return s.save()
	}
	return ErrProjectNotFound
}

// SetStar 设置某个任务的星标状态。幂等：重复设置不会产生重复记录，
// 状态没变化时连盘都不写。
func (s *Store) SetStar(projectID, taskID string, starred bool) error {
	if projectID == "" || taskID == "" {
		return errors.New("项目和任务 id 不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 只校验项目在不在，不校验任务：项目内配置模式下任务来自仓库文件，
	// 这里看不到。任务级的悬空星标不会渲染也不会报错，用户重新导入配置后
	// 还能自动恢复，比直接拒绝更宽松也更符合预期。
	if !s.hasProject(projectID) {
		return ErrProjectNotFound
	}

	idx := -1
	for i, st := range s.ws.Stars {
		if st.ProjectID == projectID && st.TaskID == taskID {
			idx = i
			break
		}
	}

	switch {
	case starred && idx >= 0:
		return nil // 已经是星标，不写盘
	case !starred && idx < 0:
		return nil // 本来就不是星标，不写盘
	}

	prev := append([]StarRef(nil), s.ws.Stars...)
	if starred {
		s.ws.Stars = append(s.ws.Stars, StarRef{ProjectID: projectID, TaskID: taskID})
	} else {
		s.ws.Stars = append(s.ws.Stars[:idx], s.ws.Stars[idx+1:]...)
	}
	if err := s.save(); err != nil {
		s.ws.Stars = prev // 写盘失败就回滚内存，别让内存和磁盘各说各话
		return err
	}
	return nil
}

func (s *Store) hasProject(id string) bool {
	for _, p := range s.ws.Projects {
		if p.ID == id {
			return true
		}
	}
	return false
}

func dropStarsOf(in []StarRef, projectID string) []StarRef {
	out := make([]StarRef, 0, len(in))
	for _, st := range in {
		if st.ProjectID != projectID {
			out = append(out, st)
		}
	}
	return out
}

// Resolve 把项目解析成前端视图。
//
// 默认只读全局注册表里的任务，完全不碰仓库——这是零侵入的默认行为。
// 只有用户显式打开 UseRepoConfig，才会去读仓库里的 .bootpane.json，
// 此时仓库配置完全取代全局配置，两者不叠加。
func (s *Store) Resolve(p Project) ResolvedProject {
	s.mu.RLock()
	stars := append([]StarRef(nil), s.ws.Stars...)
	s.mu.RUnlock()
	return s.resolve(p, stars)
}

// resolve 是 Resolve 的无锁版本，供已经持锁的调用方（Projects/Get/Add）使用。
//
// 不能让持锁路径再调 Resolve 自己再去 RLock：RWMutex 的读锁虽然可重入，
// 但只要中间排进一个等待中的写锁，第二次 RLock 就会死锁。
func (s *Store) resolve(p Project, stars []StarRef) ResolvedProject {
	rp := ResolvedProject{
		ID:             p.ID,
		Name:           p.Name,
		Description:    p.Description,
		Path:           p.Path,
		Tags:           append([]string{}, p.Tags...),
		Pinned:         p.Pinned,
		UseRepoConfig:  p.UseRepoConfig,
		ConfigSource:   SourceGlobal,
		Tasks:          []Task{},
		Pipelines:      []Pipeline{},
		Issues:         []Issue{},
		StarredTaskIDs: []string{},
	}

	// 星标按项目注入。任务已经不在配置里的悬空星标照样返回 id，
	// 前端找不到对应卡片自然就不渲染，不需要在这里做存在性过滤。
	for _, st := range stars {
		if st.ProjectID == p.ID {
			rp.StarredTaskIDs = append(rp.StarredTaskIDs, st.TaskID)
		}
	}

	if fi, err := os.Stat(p.Path); err == nil && fi.IsDir() {
		rp.Exists = true
	} else {
		rp.Issues = append(rp.Issues, Issue{Level: "error", Message: "项目目录不存在或不可访问"})
	}

	// 不管开没开项目内配置，都探一下仓库里有没有 .bootpane.json，
	// 界面上要能提示「这个仓库自带配置，要不要用」。
	var pf ProjectFile
	if rp.Exists {
		raw, err := os.ReadFile(filepath.Join(p.Path, RepoConfigName))
		switch {
		case err == nil:
			rp.HasRepoCfg = true
			if e := json.Unmarshal(raw, &pf); e != nil {
				rp.ConfigErr = fmt.Sprintf("%s 解析失败: %v", RepoConfigName, e)
				pf = ProjectFile{}
			}
		case !errors.Is(err, os.ErrNotExist):
			rp.ConfigErr = fmt.Sprintf("读取 %s 失败: %v", RepoConfigName, err)
		}
	}

	if p.UseRepoConfig {
		rp.ConfigSource = SourceRepo
		rp.Tasks = normalizeTasks(pf.Tasks)
		rp.Pipelines = nonNilPipelines(pf.Pipelines)
		if rp.Exists && !rp.HasRepoCfg {
			rp.Issues = append(rp.Issues, Issue{
				Level:   "error",
				Message: "已切换为项目内配置，但仓库里没有 " + RepoConfigName,
			})
		}
		if rp.ConfigErr != "" {
			rp.Issues = append(rp.Issues, Issue{Level: "error", Message: rp.ConfigErr})
		}
		if len(pf.Project.Tags) > 0 {
			rp.Tags = dedupe(append(rp.Tags, pf.Project.Tags...))
		}
	} else {
		rp.Tasks = normalizeTasks(p.Tasks)
		rp.Pipelines = nonNilPipelines(p.Pipelines)
	}

	if rp.Name == "" {
		rp.Name = pf.Project.Name
	}
	if rp.Name == "" {
		rp.Name = filepath.Base(p.Path)
	}
	// 描述和名字走同一条回退链：注册表里填了就用注册表，否则用仓库 meta。
	if rp.Description == "" {
		rp.Description = pf.Project.Description
	}
	if rp.Tags == nil {
		rp.Tags = []string{}
	}

	rp.Issues = append(rp.Issues, Validate(&rp)...)
	rp.PageURLs = pageURLs(rp.Tasks)
	return rp
}

// pageURLs 为每个能解析出地址的任务生成一份映射。
// 没有可打开的页面时返回 nil，序列化后前端拿到的是 null 而不是空对象。
func pageURLs(tasks []Task) map[string]string {
	out := make(map[string]string, len(tasks))
	for _, t := range tasks {
		if u := t.PageURL(); u != "" {
			out[t.ID] = u
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Settings 返回本机偏好的副本。
//
// 返回副本而不是指针：调用方（前端服务）不该有机会绕过 SetXxx 直接改内存。
func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ws.Settings
}

// SetAutoOpenLogOnStart 设置「启动任务后自动打开日志」。
//
// 幂等：值没变化就不写盘。写盘失败回滚内存，避免内存和磁盘各说各话。
func (s *Store) SetAutoOpenLogOnStart(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := s.ws.Settings.AutoOpenLogOnStart
	if prev == enabled {
		return nil
	}
	s.ws.Settings.AutoOpenLogOnStart = enabled
	if err := s.save(); err != nil {
		s.ws.Settings.AutoOpenLogOnStart = prev
		return err
	}
	return nil
}

// SetOpenWith 设置「打开目录」使用的命令，空字符串表示回到系统文件管理器。
//
// 和 SetAutoOpenLogOnStart 一样幂等：值没变化就不写盘，写盘失败回滚内存。
func (s *Store) SetOpenWith(command string) error {
	command = strings.TrimSpace(command)

	s.mu.Lock()
	defer s.mu.Unlock()

	prev := s.ws.Settings.OpenWith
	if prev == command {
		return nil
	}
	s.ws.Settings.OpenWith = command
	if err := s.save(); err != nil {
		s.ws.Settings.OpenWith = prev
		return err
	}
	return nil
}

// SetUseRepoConfig 切换某个项目的配置来源。
func (s *Store) SetUseRepoConfig(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.ws.Projects {
		if s.ws.Projects[i].ID != id {
			continue
		}
		prev := s.ws.Projects[i].UseRepoConfig
		s.ws.Projects[i].UseRepoConfig = enabled
		if err := s.save(); err != nil {
			s.ws.Projects[i].UseRepoConfig = prev
			return err
		}
		return nil
	}
	return ErrProjectNotFound
}

// SetTasks 覆盖某个项目在全局注册表里的任务定义。
func (s *Store) SetTasks(id string, tasks []Task, pipelines []Pipeline) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.ws.Projects {
		if s.ws.Projects[i].ID != id {
			continue
		}
		prevTasks, prevPipelines := s.ws.Projects[i].Tasks, s.ws.Projects[i].Pipelines
		s.ws.Projects[i].Tasks = tasks
		s.ws.Projects[i].Pipelines = pipelines
		if err := s.save(); err != nil {
			s.ws.Projects[i].Tasks, s.ws.Projects[i].Pipelines = prevTasks, prevPipelines
			return err
		}
		return nil
	}
	return ErrProjectNotFound
}

// ConfigSnapshot 返回当前生效配置的完整 JSON，供界面直接编辑。
//
// 项目内配置模式下返回仓库文件的原文——用户可能自己排过版，没必要先
// 反序列化再重新序列化一遍把格式搅乱。文件不存在时给一份空骨架，
// 让「切到项目内配置但还没建文件」的状态也能直接在界面上写出来。
//
// 全局模式则从注册表拼出同样结构的 ProjectFile。两种模式下编辑框长得一样，
// 用户不用记「我现在改的到底是哪份文件」。
func (s *Store) ConfigSnapshot(id string) (string, error) {
	s.mu.RLock()
	var p Project
	found := false
	for _, item := range s.ws.Projects {
		if item.ID == id {
			p, found = item, true
			break
		}
	}
	s.mu.RUnlock()

	if !found {
		return "", ErrProjectNotFound
	}

	if p.UseRepoConfig {
		raw, err := os.ReadFile(filepath.Join(p.Path, RepoConfigName))
		switch {
		case err == nil:
			return string(raw), nil
		case errors.Is(err, os.ErrNotExist):
			return marshalProjectFile(ProjectFile{
				Version: 1,
				Project: Meta{Name: p.Name, Description: p.Description, Tags: p.Tags},
			})
		default:
			return "", fmt.Errorf("读取 %s 失败: %w", RepoConfigName, err)
		}
	}

	return marshalProjectFile(ProjectFile{
		Version:   1,
		Project:   Meta{Name: p.Name, Description: p.Description, Tags: p.Tags},
		Tasks:     p.Tasks,
		Pipelines: p.Pipelines,
	})
}

// ApplyConfig 把解析好的配置写回当前生效的配置源。
//
// 全局模式写注册表，项目内模式写仓库里的 .bootpane.json——两条路走同一个入口，
// 免得界面上的编辑和「导入」出现两套校验、两套行为。
//
// syncMeta 控制 project 里的元信息怎么写回注册表：编辑路径传 true，名称/描述/标签
// 都以传入的配置为准（允许清空）；导入路径传 false，名称和描述取 JSON 里给出的非空值，
// 标签不动。导入也要写名称和描述，否则 prompt 里要求的字段等于白写——模型给了却不用，
// 用户导入后看到的还是旧名字、旧描述。空值视为「模型没给」，保留注册表原值。
func (s *Store) ApplyConfig(id string, pf ProjectFile, syncMeta bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	for i := range s.ws.Projects {
		if s.ws.Projects[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrProjectNotFound
	}
	p := &s.ws.Projects[idx]

	if p.UseRepoConfig {
		return writeRepoFile(p.Path, pf)
	}

	prevTasks, prevPipelines := p.Tasks, p.Pipelines
	prevName, prevTags, prevDesc := p.Name, p.Tags, p.Description

	p.Tasks = pf.Tasks
	p.Pipelines = pf.Pipelines
	if syncMeta {
		// 编辑框里显示了项目名和标签，保存后就该是这个值——否则编辑框是「假的」。
		// 空名字回落到原值：注册表里的 Name 是侧栏显示用的，不能为空。
		if name := strings.TrimSpace(pf.Project.Name); name != "" {
			p.Name = name
		}
		p.Tags = pf.Project.Tags
		p.Description = strings.TrimSpace(pf.Project.Description)
	} else {
		// 导入路径：模型在 JSON 里给出的名称和描述要落到项目上，否则界面上
		// 看不到这次生成的元信息。空值当作「没给」，保留原值——prompt 允许
		// 省略 description，省略时不该把用户手写的那句清掉。
		if name := strings.TrimSpace(pf.Project.Name); name != "" {
			p.Name = name
		}
		if desc := strings.TrimSpace(pf.Project.Description); desc != "" {
			p.Description = desc
		}
	}

	if err := s.save(); err != nil {
		p.Tasks, p.Pipelines = prevTasks, prevPipelines
		p.Name, p.Tags, p.Description = prevName, prevTags, prevDesc
		return err
	}
	return nil
}

// marshalProjectFile 把配置序列化成界面上的编辑格式。
//
// Tasks 为 nil 时 JSON 会输出 null，编辑框里看到 "tasks": null 会让人以为
// 配置坏了，所以统一补成 []。
func marshalProjectFile(pf ProjectFile) (string, error) {
	if pf.Version == 0 {
		pf.Version = 1
	}
	if pf.Tasks == nil {
		pf.Tasks = []Task{}
	}
	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

// writeRepoFile 原子写入仓库里的 .bootpane.json。
//
// 先写临时文件再重命名：这个文件在用户的工作区里，很可能正被 git 或编辑器
// 盯着，中途崩溃留下半截 JSON 比写失败更糟。
func writeRepoFile(dir string, pf ProjectFile) error {
	body, err := marshalProjectFile(pf)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, RepoConfigName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", RepoConfigName, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("写入 %s 失败: %w", RepoConfigName, err)
	}
	return nil
}

// normalizeTasks 给缺 label 的任务补上默认显示名。
func normalizeTasks(in []Task) []Task {
	out := make([]Task, 0, len(in))
	for _, t := range in {
		if t.Label == "" {
			t.Label = t.ID
		}
		out = append(out, t)
	}
	return out
}

func nonNilPipelines(in []Pipeline) []Pipeline {
	if in == nil {
		return []Pipeline{}
	}
	return in
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
