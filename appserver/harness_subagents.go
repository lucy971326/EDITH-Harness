package appserver

import (
	"context"

	"harness/appserver/internal/clientconn"
	"harness/kernel/events"
	"harness/kernel/runner"
	"harness/products/harness"
)

func (s *Server) handleSubagentList(_ context.Context, input SubagentListParams) (SubagentListResult, error) {
	tasks, err := s.harnessProduct.SubagentList(input.ParentSessionID)
	if tasks == nil {
		tasks = []harness.SubagentInfo{}
	}
	return SubagentListResult{Tasks: tasks}, methodError(err)
}

func (s *Server) handleSubagentSubscribe(ctx context.Context, input SubagentParams) (SubagentSubscribeResult, error) {
	request, err := clientconn.FromContext(ctx)
	if err != nil {
		return SubagentSubscribeResult{}, err
	}
	subscription, err := request.Subscribe()
	if err != nil {
		return SubagentSubscribeResult{}, err
	}
	snapshot, err := s.harnessProduct.SubagentSnapshot(input.ParentSessionID, input.TaskID)
	if err != nil {
		subscription.Close()
		return SubagentSubscribeResult{}, methodError(err)
	}
	listener := &runListener{sessionID: snapshot.ChildSessionID, subscription: subscription, pending: make(map[uint64]runner.RunEvent)}
	unlisten, err := events.Subscribe(s.events, listener.receive)
	if err != nil {
		subscription.Close()
		return SubagentSubscribeResult{}, err
	}
	subscription.SetCleanup(unlisten)
	// 安装监听后再重取快照，补上首次查询与监听之间的变化。
	snapshot, err = s.harnessProduct.SubagentSnapshot(input.ParentSessionID, input.TaskID)
	if err != nil {
		subscription.Close()
		return SubagentSubscribeResult{}, methodError(err)
	}
	listener.start(snapshot.Snapshot)
	return SubagentSubscribeResult{
		SubscriptionID: subscription.ID(), Task: snapshot.Task, Snapshot: snapshot.Snapshot,
		ChildSessionID: snapshot.ChildSessionID,
	}, nil
}

func (s *Server) handleSubagentSend(ctx context.Context, input SubagentSendParams) (SubagentSendResult, error) {
	message, err := messageFromInput(input.Text, input.Images)
	if err != nil {
		return SubagentSendResult{}, err
	}
	result, err := s.harnessProduct.SendSubagent(ctx, input.ParentSessionID, input.TaskID, message)
	mode := "started"
	if result.Steered {
		mode = "steered"
	}
	return SubagentSendResult{Mode: mode, Turn: result.Turn, RunID: result.RunID}, methodError(err)
}

func (s *Server) handleSubagentSettings(ctx context.Context, input SubagentSettingsParams) (SubagentSettingsResult, error) {
	_, err := s.harnessProduct.UpdateSubagentSettings(ctx, input.ParentSessionID, input.TaskID, input.Model, input.ReasoningEffort)
	if err != nil {
		return SubagentSettingsResult{}, methodError(err)
	}
	snapshot, err := s.harnessProduct.SubagentSnapshot(input.ParentSessionID, input.TaskID)
	return SubagentSettingsResult{Task: snapshot.Task}, methodError(err)
}

func (s *Server) handleSubagentStop(ctx context.Context, input SubagentParams) (StopResult, error) {
	return StopResult{}, methodError(s.harnessProduct.StopSubagent(ctx, input.ParentSessionID, input.TaskID))
}

func (s *Server) handleReadSubagentRunDiff(_ context.Context, input ReadSubagentRunDiffParams) (ReadRunDiffResult, error) {
	result, err := s.harnessProduct.ReadSubagentRunDiff(input.ParentSessionID, input.TaskID, input.RunID, input.Path)
	return result, methodError(err)
}

func (s *Server) handleRevertSubagentRunDiff(_ context.Context, input RevertSubagentRunDiffParams) (RevertRunDiffResult, error) {
	result, err := s.harnessProduct.RevertSubagentRunDiff(input.ParentSessionID, input.TaskID, input.RunID, input.Path, input.ExpectedRevision)
	return result, methodError(err)
}
