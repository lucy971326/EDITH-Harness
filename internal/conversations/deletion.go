package conversations

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"harness/internal/approvals"
	"harness/internal/hooks"
	"harness/internal/persist"
	"harness/internal/session"
)

// 数据。删除意图先于任何文件清理持久化；重启按同一清单幂等重放。
type deletionIntent struct {
	Version       int      `json:"version"`
	Workspace     string   `json:"workspace,omitempty"`
	HookWorkspace string   `json:"hookWorkspace,omitempty"`
	SessionIDs    []string `json:"sessionIDs"`
	TaskIDs       []string `json:"taskIDs"`
}

func deletionFiles(files *persist.Files) (*persist.Files, error) {
	return files.Scope("deletions")
}

// 第二个返回值表示意图文件已经存在，或写入结果无法确认；此时必须冻结会话。
func saveDeletion(files *persist.Files, intent deletionIntent) (string, bool, error) {
	return saveDeletionWithWrite(files, intent, (*persist.Files).Write)
}

func saveDeletionWithWrite(files *persist.Files, intent deletionIntent, write func(*persist.Files, string, []byte) error) (string, bool, error) {
	if err := validDeletion(intent); err != nil {
		return "", false, err
	}
	id, err := session.NewID()
	if err != nil {
		return "", false, err
	}
	body, err := json.Marshal(intent)
	if err != nil {
		return "", false, err
	}
	dir, err := deletionFiles(files)
	if err != nil {
		return "", false, err
	}
	name := id + ".json"
	if err := write(dir, name, body); err != nil {
		// Write 可能已完成重命名，只在同步目录时失败。无法确认意图不存在时，
		// 本进程先冻结目标，交由下次启动重放或恢复，不能继续接受新写入。
		_, readErr := dir.Read(name)
		if errors.Is(readErr, os.ErrNotExist) {
			return "", false, fmt.Errorf("conversation: persist deletion: %w", err)
		}
		return id, true, fmt.Errorf("conversation: deletion intent needs startup recovery: %w", errors.Join(err, readErr))
	}
	return id, true, nil
}

func validDeletion(intent deletionIntent) error {
	if intent.Version != 1 || len(intent.SessionIDs) == 0 {
		return fmt.Errorf("conversation: invalid deletion intent")
	}
	if intent.Workspace != "" && !filepath.IsAbs(intent.Workspace) {
		return fmt.Errorf("conversation: invalid deletion workspace")
	}
	for _, id := range append(append([]string(nil), intent.SessionIDs...), intent.TaskIDs...) {
		if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `:/\`) {
			return fmt.Errorf("conversation: invalid deletion ID %q", id)
		}
	}
	return nil
}

// RecoverDeletions 在子任务关系恢复之前重放未完成删除。
func RecoverDeletions(files *persist.Files) error {
	dir, err := deletionFiles(files)
	if err != nil {
		return err
	}
	entries, err := dir.List()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir || !strings.HasSuffix(entry.Name, ".json") {
			continue
		}
		body, err := dir.Read(entry.Name)
		if err != nil {
			return err
		}
		var intent deletionIntent
		if err := json.Unmarshal(body, &intent); err != nil {
			return fmt.Errorf("conversation: decode deletion %q: %w", entry.Name, err)
		}
		if err := validDeletion(intent); err != nil {
			return err
		}
		if err := finishDeletion(files, entry.Name, intent); err != nil {
			return err
		}
	}
	return nil
}

func finishDeletion(files *persist.Files, name string, intent deletionIntent) error {
	tasks, err := files.Scope("subagents", "tasks")
	if err != nil {
		return err
	}
	for _, id := range intent.TaskIDs {
		if err := tasks.Remove(id + ".json"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("conversation: remove task %q: %w", id, err)
		}
	}
	sessions, err := files.Scope("sessions")
	if err != nil {
		return err
	}
	for _, id := range intent.SessionIDs {
		if err := sessions.RemoveDir(id); err != nil {
			return fmt.Errorf("conversation: remove session %q: %w", id, err)
		}
	}
	if intent.Workspace != "" {
		if err := approvals.ForgetWorkspaceTrust(files, intent.Workspace); err != nil {
			return err
		}
		canonical := intent.HookWorkspace
		if canonical == "" {
			canonical = intent.Workspace
		}
		if err := hooks.ForgetProjectTrust(files, canonical); err != nil {
			return err
		}
	}
	dir, err := deletionFiles(files)
	if err != nil {
		return err
	}
	return dir.Remove(name)
}

// Archive 只改元数据；运行中的轮次继续，下一轮需恢复后才能启动。
func (s *Service) Archive(id string) (SessionInfo, error) {
	op, err := s.operation(id)
	if err != nil {
		return SessionInfo{}, err
	}
	op.Lock()
	defer op.Unlock()
	info, err := s.Session(id)
	if err != nil {
		return SessionInfo{}, err
	}
	info.Meta, err = s.sessions.SetArchived(id, true)
	return info, err
}

func (s *Service) Restore(id string) (SessionInfo, error) {
	op, err := s.operation(id)
	if err != nil {
		return SessionInfo{}, err
	}
	op.Lock()
	defer op.Unlock()
	info, err := s.Session(id)
	if err != nil {
		return SessionInfo{}, err
	}
	info.Meta, err = s.sessions.SetArchived(id, false)
	return info, err
}

// Archived 返回根会话中的已归档项。
func (s *Service) Archived() ([]SessionInfo, error) {
	return s.list(true)
}

// DeleteSession 永久删除一个根会话及其子任务；忙碌时返回冲突。
func (s *Service) DeleteSession(id string) ([]string, error) {
	s.createMu.Lock()
	defer s.createMu.Unlock()
	op, err := s.operation(id)
	if err != nil {
		return nil, err
	}
	if !op.TryLock() {
		return nil, ErrRunActive
	}
	defer op.Unlock()
	if _, err := s.Session(id); err != nil {
		return nil, err
	}
	return s.deleteRoots([]string{id}, "")
}

// DeleteProject 按完整工作区路径删除所有根会话，包括已归档项。
func (s *Service) DeleteProject(workspace string) ([]string, error) {
	if !filepath.IsAbs(workspace) {
		return nil, ErrWorkspace
	}
	s.createMu.Lock()
	defer s.createMu.Unlock()
	infos, err := s.allRoots()
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0)
	for _, info := range infos {
		if info.Settings.Workspace == workspace {
			roots = append(roots, info.Meta.ID)
		}
	}
	if len(roots) == 0 {
		return nil, ErrSessionNotFound
	}
	sort.Strings(roots)
	locked := make([]*sync.Mutex, 0, len(roots))
	for _, id := range roots {
		op, err := s.operation(id)
		if err != nil {
			for _, held := range locked {
				held.Unlock()
			}
			return nil, err
		}
		if !op.TryLock() {
			for _, held := range locked {
				held.Unlock()
			}
			return nil, ErrRunActive
		}
		locked = append(locked, op)
	}
	defer func() {
		for _, held := range locked {
			held.Unlock()
		}
	}()
	return s.deleteRoots(roots, workspace)
}

func (s *Service) deleteRoots(roots []string, workspace string) ([]string, error) {
	var name string
	var intent deletionIntent
	var writeErr error
	ids, _, err := s.subagents.ReserveDeletion(roots, func(sessionIDs, taskIDs []string) error {
		intent = deletionIntent{Version: 1, Workspace: workspace, SessionIDs: sessionIDs, TaskIDs: taskIDs}
		if workspace != "" {
			intent.HookWorkspace = workspace
			if real, resolveErr := filepath.EvalSymlinks(workspace); resolveErr == nil {
				intent.HookWorkspace = real
			}
		}
		var pending bool
		name, pending, writeErr = saveDeletion(s.files, intent)
		if !pending {
			return writeErr
		}
		s.deletedMu.Lock()
		for _, id := range sessionIDs {
			s.deleted[id] = struct{}{}
		}
		s.deletedMu.Unlock()
		return nil
	})
	if err != nil {
		return nil, err
	}
	if writeErr != nil {
		return nil, writeErr
	}
	if err := finishDeletion(s.files, name+".json", intent); err != nil {
		return nil, err
	}
	s.sessions.Forget(ids)
	s.runner.ForgetSessions(ids)
	for _, id := range ids {
		s.operations.Delete(id)
	}
	return ids, nil
}
