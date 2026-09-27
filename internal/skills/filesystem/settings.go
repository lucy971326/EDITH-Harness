package filesystem

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"harness/internal/machine"
	kernskills "harness/internal/skills"
)

const maxSkillBytes = 1 << 20

type skillState struct {
	Disabled []string `json:"disabled"`
}

func (p *Provider) disabled() (map[string]bool, error) {
	data, err := p.files.Read("skills.json")
	if errors.Is(err, os.ErrNotExist) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	var state skillState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("skills-filesystem: invalid skills.json: %w", err)
	}
	disabled := make(map[string]bool, len(state.Disabled))
	for _, name := range state.Disabled {
		disabled[name] = true
	}
	return disabled, nil
}

func (p *Provider) writeDisabled(disabled map[string]bool) error {
	state := skillState{Disabled: make([]string, 0, len(disabled))}
	for name, off := range disabled {
		if off {
			state.Disabled = append(state.Disabled, name)
		}
	}
	sort.Strings(state.Disabled)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return p.files.Write("skills.json", append(data, '\n'))
}

// Catalog 按运行时发现顺序列出所有来源；单个损坏条目不会阻止设置页。
func (p *Provider) Catalog(workspace string) ([]kernskills.SettingsItem, error) {
	roots, err := p.roots(workspace)
	if err != nil {
		return nil, err
	}
	disabled, err := p.disabled()
	if err != nil {
		return nil, err
	}
	items := make([]kernskills.SettingsItem, 0)
	seen := make(map[string]bool)
	for _, candidate := range roots {
		entries, err := p.readRoot(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
		for _, entry := range entries {
			if !entry.IsDir {
				continue
			}
			path, err := p.skillPath(candidate, entry.Name)
			if err != nil {
				return nil, err
			}
			item := kernskills.SettingsItem{
				Name: entry.Name, Source: candidate.source, Path: path,
				Enabled:    candidate.source != "personal" || !disabled[entry.Name],
				Overridden: seen[entry.Name],
			}
			skill, ok, err := p.readSkill(candidate, entry.Name)
			if err != nil {
				item.Error = err.Error()
			}
			if !ok && err == nil {
				item.Error = "SKILL.md 不存在"
			}
			if err == nil && ok {
				item.Description = skill.Description
			}
			items = append(items, item)
			// 关闭的个人 Skill 也占据其名称，不能从较低优先级的 .agents 回退。
			if ok || candidate.source == "personal" && disabled[entry.Name] {
				seen[entry.Name] = true
			}
		}
	}
	if p.system != nil {
		system, err := p.system.List(workspace)
		if err != nil {
			return nil, err
		}
		for _, skill := range system {
			items = append(items, kernskills.SettingsItem{
				Name: skill.Name, Description: skill.Description, Source: "system",
				Path: skill.Location, Enabled: true,
			})
		}
	}
	return items, nil
}

func (p *Provider) skillPath(candidate root, name string) (string, error) {
	if candidate.files != nil {
		return candidate.files.Path(name, "SKILL.md")
	}
	return p.machine.ResolvePath(candidate.path, filepath.Join(name, "SKILL.md")), nil
}

// ReadDocument 只按已知来源和目录名定位文件，不接受客户端传入的任意路径。
func (p *Provider) ReadDocument(workspace, source, name string) (kernskills.Document, error) {
	if err := validateName(name); err != nil {
		return kernskills.Document{}, fmt.Errorf("%w: %v", kernskills.ErrInvalid, err)
	}
	path := ""
	if source == "system" && p.system != nil {
		entries, err := p.system.List(workspace)
		if err != nil {
			return kernskills.Document{}, err
		}
		for _, entry := range entries {
			if entry.Name == name {
				path = entry.Location
				break
			}
		}
	} else {
		roots, err := p.roots(workspace)
		if err != nil {
			return kernskills.Document{}, err
		}
		for _, candidate := range roots {
			if candidate.source == source {
				path, err = p.skillPath(candidate, name)
				if err != nil {
					return kernskills.Document{}, err
				}
				break
			}
		}
	}
	if path == "" {
		return kernskills.Document{}, kernskills.ErrNotFound
	}
	if source == "personal" {
		if err := checkPersonalPath(path); err != nil {
			return kernskills.Document{}, err
		}
	}
	data, err := p.machine.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if source != "personal" {
			return kernskills.Document{}, kernskills.ErrNotFound
		}
		data = nil
	} else if err != nil {
		return kernskills.Document{}, err
	}
	if len(data) > maxSkillBytes {
		return kernskills.Document{}, fmt.Errorf("%w: SKILL.md 超过 1 MiB", kernskills.ErrInvalid)
	}
	version := ""
	if data != nil {
		version = hashSource(data)
	}
	resources := []string{}
	if entries, listErr := p.machine.ReadDir(filepath.Dir(path)); listErr == nil {
		for _, entry := range entries {
			if entry.Name != "SKILL.md" {
				resources = append(resources, entry.Name)
			}
		}
		sort.Strings(resources)
	}
	return kernskills.Document{Content: string(data), Version: version, Resources: resources}, nil
}

func hashSource(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func checkPersonalPath(path string) error {
	info, err := os.Lstat(filepath.Dir(path))
	if errors.Is(err, os.ErrNotExist) {
		return kernskills.ErrNotFound
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: Skill 目录不是普通目录", kernskills.ErrInvalid)
	}
	info, err = os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: SKILL.md 不是普通文件", kernskills.ErrInvalid)
	}
	return nil
}

// Save 按文件版本保存个人 SKILL.md；源码的额外 frontmatter 字段原样保留。
func (p *Provider) Save(name, content, version string, create bool) (kernskills.Document, error) {
	if err := validateName(name); err != nil {
		return kernskills.Document{}, fmt.Errorf("%w: %v", kernskills.ErrInvalid, err)
	}
	if len(content) > maxSkillBytes {
		return kernskills.Document{}, fmt.Errorf("%w: SKILL.md 超过 1 MiB", kernskills.ErrInvalid)
	}
	parsedName, description, err := parseFrontmatter([]byte(content))
	if err != nil {
		return kernskills.Document{}, fmt.Errorf("%w: %v", kernskills.ErrInvalid, err)
	}
	if parsedName != name {
		return kernskills.Document{}, fmt.Errorf("%w: frontmatter 名称必须与目录名一致", kernskills.ErrInvalid)
	}
	if err := validateDescription(description); err != nil {
		return kernskills.Document{}, fmt.Errorf("%w: %v", kernskills.ErrInvalid, err)
	}
	filesystem, ok := p.machine.(machine.FileSystem)
	if !ok {
		return kernskills.Document{}, fmt.Errorf("skills-filesystem: machine does not support versioned writes")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	personal, err := p.files.Scope("skills")
	if err != nil {
		return kernskills.Document{}, err
	}
	path, err := personal.Path(name, "SKILL.md")
	if err != nil {
		return kernskills.Document{}, err
	}
	if create {
		if version != "" {
			return kernskills.Document{}, kernskills.ErrConflict
		}
		if p.system != nil {
			system, err := p.system.List("")
			if err != nil {
				return kernskills.Document{}, err
			}
			for _, skill := range system {
				if skill.Name == name {
					return kernskills.Document{}, fmt.Errorf("%w: 与内置 Skill 同名", kernskills.ErrInvalid)
				}
			}
		}
		// 删除后保留的关闭记录不应影响同名新 Skill；先清理状态，再创建目录。
		disabled, err := p.disabled()
		if err != nil {
			return kernskills.Document{}, err
		}
		if disabled[name] {
			delete(disabled, name)
			if err := p.writeDisabled(disabled); err != nil {
				return kernskills.Document{}, err
			}
		}
		if err := personal.Ensure(); err != nil {
			return kernskills.Document{}, err
		}
		dir, err := personal.Path(name)
		if err != nil {
			return kernskills.Document{}, err
		}
		if err := os.Mkdir(dir, 0o700); err != nil {
			if errors.Is(err, os.ErrExist) {
				return kernskills.Document{}, kernskills.ErrConflict
			}
			return kernskills.Document{}, err
		}
	} else {
		if err := checkPersonalPath(path); err != nil {
			return kernskills.Document{}, err
		}
	}
	_, err = filesystem.WriteFileIfUnchanged(path, []byte(content), version)
	if err != nil {
		if create {
			_ = os.Remove(filepath.Dir(path))
		}
		if errors.Is(err, machine.ErrFileConflict) {
			return kernskills.Document{}, kernskills.ErrConflict
		}
		if errors.Is(err, os.ErrNotExist) {
			return kernskills.Document{}, kernskills.ErrNotFound
		}
		return kernskills.Document{}, err
	}
	return kernskills.Document{Content: content, Version: hashSource([]byte(content)), Resources: []string{}}, nil
}

// SetEnabled 只保存个人 Skill 的状态，不修改 SKILL.md。
func (p *Provider) SetEnabled(name string, enabled bool) error {
	if err := validateName(name); err != nil {
		return fmt.Errorf("%w: %v", kernskills.ErrInvalid, err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	personal, err := p.files.Scope("skills")
	if err != nil {
		return err
	}
	path, err := personal.Path(name, "SKILL.md")
	if err != nil {
		return err
	}
	if err := checkPersonalPath(path); err != nil {
		return err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return kernskills.ErrNotFound
	} else if err != nil {
		return err
	}
	disabled, err := p.disabled()
	if err != nil {
		return err
	}
	if enabled {
		delete(disabled, name)
	} else {
		disabled[name] = true
	}
	return p.writeDisabled(disabled)
}

// Delete 删除个人 Skill 目录及附属资源，版本不符时拒绝。
func (p *Provider) Delete(name, version string) error {
	if err := validateName(name); err != nil {
		return fmt.Errorf("%w: %v", kernskills.ErrInvalid, err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	personal, err := p.files.Scope("skills")
	if err != nil {
		return err
	}
	path, err := personal.Path(name, "SKILL.md")
	if err != nil {
		return err
	}
	if err := checkPersonalPath(path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		data = nil
	} else if err != nil {
		return err
	}
	actual := ""
	if data != nil {
		actual = hashSource(data)
	}
	if !strings.EqualFold(actual, version) {
		return kernskills.ErrConflict
	}
	if err := personal.RemoveDir(name); err != nil {
		return err
	}
	return nil
}

var _ kernskills.Settings = (*Provider)(nil)
