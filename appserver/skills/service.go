package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"
)

const maxSkillFileBytes = 1 << 20

var (
	ErrPluginNotFound    = errors.New("appserver/skills: plugin not found")
	ErrSkillNotFound     = errors.New("appserver/skills: skill not found")
	ErrPathOutsideRoot   = errors.New("appserver/skills: path escapes configured root")
	ErrConfiguredRoot    = errors.New("appserver/skills: configured root is invalid")
	ErrSkillFileTooLarge = errors.New("appserver/skills: skill file exceeds maximum size")
)

type Option func(*Service)

func WithRoot(path string) Option {
	return func(s *Service) {
		if strings.TrimSpace(path) != "" {
			s.roots = append(s.roots, strings.TrimSpace(path))
		}
	}
}

func WithRoots(paths ...string) Option {
	return func(s *Service) {
		for _, path := range paths {
			if strings.TrimSpace(path) != "" {
				s.roots = append(s.roots, strings.TrimSpace(path))
			}
		}
	}
}

type Service struct {
	mu    sync.RWMutex
	roots []string
}

type Root struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type Skill struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	Description  string `json:"description,omitempty"`
	Path         string `json:"path"`
	RelativePath string `json:"relativePath"`
	RootID       string `json:"rootId"`
	PluginID     string `json:"pluginId,omitempty"`
	PluginName   string `json:"pluginName,omitempty"`
	Source       string `json:"source"`
	Hidden       bool   `json:"hidden,omitempty"`
}

type SkillSummary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	Description  string `json:"description,omitempty"`
	Path         string `json:"path"`
	RelativePath string `json:"relativePath"`
}

type Plugin struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	DisplayName  string         `json:"displayName"`
	Description  string         `json:"description,omitempty"`
	Version      string         `json:"version,omitempty"`
	Path         string         `json:"path"`
	RelativePath string         `json:"relativePath"`
	ManifestPath string         `json:"manifestPath"`
	RootID       string         `json:"rootId"`
	Source       string         `json:"source"`
	SkillCount   int            `json:"skillCount"`
	Skills       []SkillSummary `json:"skills,omitempty"`
	Hidden       bool           `json:"hidden,omitempty"`
}

type ListParams struct {
	IncludeHidden bool     `json:"includeHidden,omitempty"`
	RootID        string   `json:"rootId,omitempty"`
	RootIDs       []string `json:"rootIds,omitempty"`
}

type ListResponse struct {
	Skills []Skill `json:"skills"`
	Data   []Skill `json:"data"`
	Roots  []Root  `json:"roots"`
}

type PluginListParams struct {
	IncludeHidden bool `json:"includeHidden,omitempty"`
	IncludeSkills bool `json:"includeSkills,omitempty"`
}

type PluginListResponse struct {
	Plugins []Plugin `json:"plugins"`
	Data    []Plugin `json:"data"`
	Roots   []Root   `json:"roots"`
}

type PluginReadParams struct {
	ID       string `json:"id,omitempty"`
	PluginID string `json:"pluginId,omitempty"`
	Name     string `json:"name,omitempty"`
	Path     string `json:"path,omitempty"`
}

type PluginReadResponse struct {
	Plugin       Plugin          `json:"plugin"`
	Manifest     map[string]any  `json:"manifest,omitempty"`
	ManifestJSON json.RawMessage `json:"manifestJson,omitempty"`
}

type PluginSkillReadParams struct {
	ID       string `json:"id,omitempty"`
	SkillID  string `json:"skillId,omitempty"`
	Name     string `json:"name,omitempty"`
	Path     string `json:"path,omitempty"`
	PluginID string `json:"pluginId,omitempty"`
}

type PluginSkillReadResponse struct {
	Skill   Skill  `json:"skill"`
	Content string `json:"content"`
}

type pluginManifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Hidden      bool   `json:"hidden"`
}

type rootInfo struct {
	Root
	eval string
}

type pluginEntry struct {
	plugin   Plugin
	manifest map[string]any
	raw      json.RawMessage
	dir      string
	root     rootInfo
}

type skillEntry struct {
	skill   Skill
	content string
	root    rootInfo
}

type snapshot struct {
	roots   []Root
	plugins []pluginEntry
	skills  []skillEntry
}

func NewService(opts ...Option) *Service {
	s := &Service{}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) Roots() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.roots...)
}

func (s *Service) ListSkills(ctx context.Context, params ListParams) (ListResponse, error) {
	snap, err := s.discover(ctx)
	if err != nil {
		return ListResponse{}, err
	}
	rootFilter := rootFilter(params.RootID, params.RootIDs)
	skills := make([]Skill, 0, len(snap.skills))
	for _, entry := range snap.skills {
		if len(rootFilter) > 0 {
			if _, ok := rootFilter[entry.skill.RootID]; !ok {
				continue
			}
		}
		if entry.skill.Hidden && !params.IncludeHidden {
			continue
		}
		skills = append(skills, cloneSkill(entry.skill))
	}
	return ListResponse{
		Skills: skills,
		Data:   cloneSkills(skills),
		Roots:  cloneRoots(snap.roots),
	}, nil
}

func (s *Service) ListPlugins(ctx context.Context, params PluginListParams) (PluginListResponse, error) {
	snap, err := s.discover(ctx)
	if err != nil {
		return PluginListResponse{}, err
	}
	plugins := make([]Plugin, 0, len(snap.plugins))
	for _, entry := range snap.plugins {
		if entry.plugin.Hidden && !params.IncludeHidden {
			continue
		}
		plugin := clonePlugin(entry.plugin)
		if !params.IncludeSkills {
			plugin.Skills = nil
		}
		plugins = append(plugins, plugin)
	}
	return PluginListResponse{
		Plugins: plugins,
		Data:    clonePlugins(plugins),
		Roots:   cloneRoots(snap.roots),
	}, nil
}

func (s *Service) ReadPlugin(ctx context.Context, params PluginReadParams) (PluginReadResponse, error) {
	snap, err := s.discover(ctx)
	if err != nil {
		return PluginReadResponse{}, err
	}
	entry, ok := snap.findPlugin(params)
	if !ok {
		return PluginReadResponse{}, ErrPluginNotFound
	}
	return PluginReadResponse{
		Plugin:       clonePlugin(entry.plugin),
		Manifest:     cloneMap(entry.manifest),
		ManifestJSON: append(json.RawMessage(nil), entry.raw...),
	}, nil
}

func (s *Service) ReadPluginSkill(ctx context.Context, params PluginSkillReadParams) (PluginSkillReadResponse, error) {
	snap, err := s.discover(ctx)
	if err != nil {
		return PluginSkillReadResponse{}, err
	}
	entry, ok := snap.findSkill(params)
	if !ok {
		return PluginSkillReadResponse{}, ErrSkillNotFound
	}
	return PluginSkillReadResponse{
		Skill:   cloneSkill(entry.skill),
		Content: entry.content,
	}, nil
}

func (s *Service) discover(ctx context.Context) (snapshot, error) {
	if err := ctxErr(ctx); err != nil {
		return snapshot{}, err
	}
	roots, err := s.rootInfos()
	if err != nil {
		return snapshot{}, err
	}
	snap := snapshot{roots: make([]Root, 0, len(roots))}
	for _, root := range roots {
		snap.roots = append(snap.roots, root.Root)
	}

	type manifestCandidate struct {
		path string
		root rootInfo
	}
	type skillCandidate struct {
		path string
		root rootInfo
	}

	var manifests []manifestCandidate
	var skills []skillCandidate
	for _, root := range roots {
		err := filepath.WalkDir(root.Path, func(path string, d fs.DirEntry, walkErr error) error {
			if err := ctxErr(ctx); err != nil {
				return err
			}
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() && shouldSkipDir(path, root.Path, d.Name()) {
				return filepath.SkipDir
			}
			if d.IsDir() {
				return nil
			}
			switch {
			case d.Name() == "SKILL.md":
				skills = append(skills, skillCandidate{path: path, root: root})
			case d.Name() == "plugin.json" && isPluginManifestPath(path, root.Path):
				manifests = append(manifests, manifestCandidate{path: path, root: root})
			}
			return nil
		})
		if err != nil {
			return snapshot{}, err
		}
	}

	usedPluginIDs := map[string]int{}
	for _, candidate := range manifests {
		entry, err := readPlugin(candidate.root, candidate.path, usedPluginIDs)
		if err != nil {
			return snapshot{}, err
		}
		snap.plugins = append(snap.plugins, entry)
	}
	slices.SortFunc(snap.plugins, func(a, b pluginEntry) int {
		return strings.Compare(a.plugin.ID, b.plugin.ID)
	})

	usedSkillIDs := map[string]int{}
	for _, candidate := range skills {
		entry, err := readSkill(candidate.root, candidate.path, snap.plugins, usedSkillIDs)
		if err != nil {
			return snapshot{}, err
		}
		snap.skills = append(snap.skills, entry)
		for i := range snap.plugins {
			if snap.plugins[i].plugin.ID == entry.skill.PluginID {
				snap.plugins[i].plugin.SkillCount++
				snap.plugins[i].plugin.Skills = append(snap.plugins[i].plugin.Skills, summarizeSkill(entry.skill))
			}
		}
	}
	slices.SortFunc(snap.skills, func(a, b skillEntry) int {
		return strings.Compare(a.skill.ID, b.skill.ID)
	})
	for i := range snap.plugins {
		slices.SortFunc(snap.plugins[i].plugin.Skills, func(a, b SkillSummary) int {
			return strings.Compare(a.ID, b.ID)
		})
	}
	return snap, nil
}

func (s *Service) rootInfos() ([]rootInfo, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.RLock()
	roots := append([]string(nil), s.roots...)
	s.mu.RUnlock()
	out := make([]rootInfo, 0, len(roots))
	seen := map[string]struct{}{}
	for _, configured := range roots {
		abs, err := filepath.Abs(configured)
		if err != nil {
			return nil, fmt.Errorf("%w: resolve %q: %w", ErrConfiguredRoot, configured, err)
		}
		abs = filepath.Clean(abs)
		if _, ok := seen[abs]; ok {
			continue
		}
		info, err := os.Stat(abs)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%w: stat %q: %w", ErrConfiguredRoot, configured, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%w: %q is not a directory", ErrConfiguredRoot, configured)
		}
		eval, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return nil, fmt.Errorf("%w: evaluate %q: %w", ErrConfiguredRoot, configured, err)
		}
		seen[abs] = struct{}{}
		out = append(out, rootInfo{
			Root: Root{
				ID:   fmt.Sprintf("root-%d", len(out)+1),
				Name: filepath.Base(abs),
				Path: abs,
			},
			eval: eval,
		})
	}
	return out, nil
}

func readPlugin(root rootInfo, path string, usedIDs map[string]int) (pluginEntry, error) {
	raw, err := readRootFile(root, path)
	if err != nil {
		return pluginEntry{}, err
	}
	var manifest pluginManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return pluginEntry{}, fmt.Errorf("parse plugin manifest %q: %w", path, err)
	}
	var manifestMap map[string]any
	if err := json.Unmarshal(raw, &manifestMap); err != nil {
		return pluginEntry{}, fmt.Errorf("parse plugin manifest map %q: %w", path, err)
	}
	dir := pluginDirForManifest(path)
	relDir := rootRelative(root, dir)
	displayName := firstNonEmpty(manifest.DisplayName, manifest.Name, filepath.Base(dir))
	id := uniqueID(slug(firstNonEmpty(manifest.ID, manifest.Name, displayName, relDir)), usedIDs)
	return pluginEntry{
		plugin: Plugin{
			ID:           id,
			Name:         firstNonEmpty(manifest.Name, displayName, id),
			DisplayName:  displayName,
			Description:  manifest.Description,
			Version:      manifest.Version,
			Path:         dir,
			RelativePath: relDir,
			ManifestPath: path,
			RootID:       root.ID,
			Source:       "filesystem",
			Hidden:       manifest.Hidden,
		},
		manifest: manifestMap,
		raw:      append(json.RawMessage(nil), raw...),
		dir:      dir,
		root:     root,
	}, nil
}

func readSkill(root rootInfo, path string, plugins []pluginEntry, usedIDs map[string]int) (skillEntry, error) {
	raw, err := readRootFile(root, path)
	if err != nil {
		return skillEntry{}, err
	}
	content := string(raw)
	rel := rootRelative(root, path)
	dir := filepath.Dir(path)
	name, description := parseSkillMetadata(content, filepath.Base(dir))
	var plugin *pluginEntry
	for i := range plugins {
		if plugins[i].root.ID == root.ID && pathInside(plugins[i].dir, dir) {
			if plugin == nil || len(plugins[i].dir) > len(plugin.dir) {
				plugin = &plugins[i]
			}
		}
	}
	idBase := slug(firstNonEmpty(name, strings.TrimSuffix(rel, "/SKILL.md")))
	if plugin != nil {
		pluginRel := relativeSlash(plugin.dir, dir)
		idBase = slug(plugin.plugin.ID + "-" + firstNonEmpty(pluginRel, name, "skill"))
	}
	id := uniqueID(idBase, usedIDs)
	skill := Skill{
		ID:           id,
		Name:         name,
		DisplayName:  name,
		Description:  description,
		Path:         path,
		RelativePath: rel,
		RootID:       root.ID,
		Source:       "filesystem",
	}
	if skill.Name == "" {
		skill.Name = id
		skill.DisplayName = id
	}
	if plugin != nil {
		skill.PluginID = plugin.plugin.ID
		skill.PluginName = plugin.plugin.DisplayName
		skill.Hidden = plugin.plugin.Hidden
	}
	return skillEntry{skill: skill, content: content, root: root}, nil
}

func (snap snapshot) findPlugin(params PluginReadParams) (pluginEntry, bool) {
	id := strings.TrimSpace(firstNonEmpty(params.PluginID, params.ID))
	name := strings.TrimSpace(params.Name)
	path := strings.TrimSpace(params.Path)
	for _, entry := range snap.plugins {
		switch {
		case id != "" && entry.plugin.ID == id:
			return entry, true
		case name != "" && (entry.plugin.Name == name || entry.plugin.DisplayName == name):
			return entry, true
		case path != "" && samePathOrRel(path, entry.plugin.Path, entry.plugin.RelativePath):
			return entry, true
		}
	}
	return pluginEntry{}, false
}

func (snap snapshot) findSkill(params PluginSkillReadParams) (skillEntry, bool) {
	id := strings.TrimSpace(firstNonEmpty(params.SkillID, params.ID))
	name := strings.TrimSpace(params.Name)
	path := strings.TrimSpace(params.Path)
	pluginID := strings.TrimSpace(params.PluginID)
	for _, entry := range snap.skills {
		if pluginID != "" && entry.skill.PluginID != pluginID {
			continue
		}
		switch {
		case id != "" && entry.skill.ID == id:
			return entry, true
		case name != "" && (entry.skill.Name == name || entry.skill.DisplayName == name):
			return entry, true
		case path != "" && samePathOrRel(path, entry.skill.Path, entry.skill.RelativePath):
			return entry, true
		}
	}
	return skillEntry{}, false
}

func readRootFile(root rootInfo, path string) ([]byte, error) {
	eval, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	if err := ensureInside(root.eval, eval); err != nil {
		return nil, err
	}
	info, err := os.Stat(eval)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxSkillFileBytes {
		return nil, fmt.Errorf("%w: %s", ErrSkillFileTooLarge, path)
	}
	return os.ReadFile(eval)
}

func isPluginManifestPath(path, root string) bool {
	dir := filepath.Dir(path)
	if filepath.Base(dir) == ".codex-plugin" {
		return true
	}
	return sameCleanPath(dir, root) || sameCleanPath(filepath.Dir(dir), root)
}

func pluginDirForManifest(path string) string {
	dir := filepath.Dir(path)
	if filepath.Base(dir) == ".codex-plugin" {
		return filepath.Dir(dir)
	}
	return dir
}

func shouldSkipDir(path, root, name string) bool {
	if path == root {
		return false
	}
	switch name {
	case ".git", "node_modules", "vendor":
		return true
	default:
		return false
	}
}

func parseSkillMetadata(content, fallback string) (string, string) {
	var name, description string
	lines := strings.Split(content, "\n")
	i := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for i = 1; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if line == "---" {
				i++
				break
			}
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "name", "title":
				name = stripScalar(value)
			case "description":
				description = stripScalar(value)
			}
		}
	}
	for ; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if name == "" {
				name = strings.TrimSpace(strings.TrimLeft(line, "#"))
			}
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), "description") && description == "" {
			description = stripScalar(value)
			continue
		}
		if description == "" {
			description = trimDescription(line)
		}
		break
	}
	if name == "" {
		name = fallback
	}
	return name, description
}

func stripScalar(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return trimDescription(value)
}

func trimDescription(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 240 {
		return strings.TrimSpace(value[:240])
	}
	return value
}

func uniqueID(base string, used map[string]int) string {
	if base == "" {
		base = "item"
	}
	n := used[base]
	used[base] = n + 1
	if n == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, n+1)
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	prevDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevDash = false
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func ensureInside(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("relativize path: %w", err)
	}
	if rel == "." {
		return nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return ErrPathOutsideRoot
	}
	return nil
}

func pathInside(root, path string) bool {
	return ensureInside(root, path) == nil
}

func samePathOrRel(input, absolute, relative string) bool {
	if filepath.IsAbs(input) {
		return sameCleanPath(input, absolute)
	}
	return filepath.ToSlash(filepath.Clean(input)) == relative
}

func sameCleanPath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func rootRelative(root rootInfo, path string) string {
	return relativeSlash(root.Path, path)
}

func relativeSlash(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func rootFilter(rootID string, rootIDs []string) map[string]struct{} {
	values := append([]string(nil), rootIDs...)
	if strings.TrimSpace(rootID) != "" {
		values = append(values, strings.TrimSpace(rootID))
	}
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out[strings.TrimSpace(value)] = struct{}{}
		}
	}
	return out
}

func summarizeSkill(skill Skill) SkillSummary {
	return SkillSummary{
		ID:           skill.ID,
		Name:         skill.Name,
		DisplayName:  skill.DisplayName,
		Description:  skill.Description,
		Path:         skill.Path,
		RelativePath: skill.RelativePath,
	}
}

func cloneRoots(in []Root) []Root {
	return append([]Root(nil), in...)
}

func cloneSkill(in Skill) Skill {
	return in
}

func cloneSkills(in []Skill) []Skill {
	return append([]Skill(nil), in...)
}

func clonePlugin(in Plugin) Plugin {
	in.Skills = append([]SkillSummary(nil), in.Skills...)
	return in
}

func clonePlugins(in []Plugin) []Plugin {
	out := make([]Plugin, 0, len(in))
	for _, plugin := range in {
		out = append(out, clonePlugin(plugin))
	}
	return out
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
