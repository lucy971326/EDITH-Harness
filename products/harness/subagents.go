package harness

import (
	"context"

	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/kernel/subagents"
)

// SubagentList 返回父会话直接创建的子任务。
func (p *Product) SubagentList(parentSessionID string) ([]SubagentInfo, error) {
	_, err := p.Session(parentSessionID)
	if err != nil {
		return nil, err
	}
	tasks, err := p.subagents.List(parentSessionID, "")
	if err != nil {
		return nil, err
	}
	result := make([]SubagentInfo, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, subagentInfo(task))
	}
	return result, nil
}

// SubagentSnapshot 返回一个子任务的页面恢复投影。
func (p *Product) SubagentSnapshot(parentSessionID, taskID string) (SubagentSnapshot, error) {
	_, err := p.Session(parentSessionID)
	if err != nil {
		return SubagentSnapshot{}, err
	}
	view, err := p.subagents.Snapshot(parentSessionID, taskID)
	if err != nil {
		return SubagentSnapshot{}, err
	}
	return SubagentSnapshot{
		Task: subagentInfo(view.Task),
		Snapshot: Snapshot{
			Entries: view.View.Entries, Runs: view.View.Runs,
			UpdateSeq: view.View.UpdateSeq, SeqEpoch: view.View.SeqEpoch,
		},
		ChildSessionID: view.Task.ChildSessionID,
	}, nil
}

// SendSubagent 由用户直接续聊指定子任务。
func (p *Product) SendSubagent(ctx context.Context, parentSessionID, taskID string, message session.UserMessage) (subagents.SendResult, error) {
	_, err := p.Session(parentSessionID)
	if err != nil {
		return subagents.SendResult{}, err
	}
	return p.subagents.SendFromUser(ctx, parentSessionID, taskID, message)
}

// UpdateSubagentSettings 在孩子空闲时更新下一轮模型和思考档位。
func (p *Product) UpdateSubagentSettings(ctx context.Context, parentSessionID, taskID, model, effort string) (settings.SessionSettings, error) {
	_, err := p.Session(parentSessionID)
	if err != nil {
		return settings.SessionSettings{}, err
	}
	return p.subagents.UpdateSettings(ctx, parentSessionID, taskID, subagents.TaskSettingsInput{
		Model: model, ReasoningEffort: effort,
	})
}

// StopSubagent 只停止指定孩子当前一轮。
func (p *Product) StopSubagent(ctx context.Context, parentSessionID, taskID string) error {
	_, err := p.Session(parentSessionID)
	if err != nil {
		return err
	}
	return p.subagents.Stop(ctx, parentSessionID, taskID)
}

// ReadSubagentRunDiff 读取指定孩子的单文件 Diff。
func (p *Product) ReadSubagentRunDiff(parentSessionID, taskID, runID, path string) (runner.RunDiffFile, error) {
	_, err := p.Session(parentSessionID)
	if err != nil {
		return runner.RunDiffFile{}, err
	}
	return p.subagents.ReadRunDiffFile(parentSessionID, taskID, runID, path)
}

// RevertSubagentRunDiff 受版本保护地撤销指定孩子改动的一个文件。
func (p *Product) RevertSubagentRunDiff(parentSessionID, taskID, runID, path string, revision uint64) (runner.RunDiffSummary, error) {
	_, err := p.Session(parentSessionID)
	if err != nil {
		return runner.RunDiffSummary{}, err
	}
	return p.subagents.RevertRunDiffFile(parentSessionID, taskID, runID, path, revision)
}

func subagentInfo(task subagents.TaskView) SubagentInfo {
	return SubagentInfo{
		TaskID: task.ID, TaskName: task.TaskName, Description: task.Description,
		AgentID: task.AgentID, Model: task.Model, ReasoningEffort: task.ReasoningEffort,
		Workspace: task.Workspace, Status: task.Status, Turn: task.Turn,
		CurrentRunID: task.CurrentRunID, Error: task.Error,
	}
}
