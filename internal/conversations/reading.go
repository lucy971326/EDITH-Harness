package conversations

import (
	"fmt"
	"harness/internal/runner"
)

// Activities 返回全部普通会话的轻量状态；审批仍从审批订阅获取。
func (s *Service) Activities() ([]SessionActivity, error) {
	sessions, err := s.List()
	if err != nil {
		return nil, err
	}
	result := make([]SessionActivity, 0, len(sessions))
	for _, info := range sessions {
		activity, err := s.activity(info.Meta.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, activity)
	}
	return result, nil
}

func (s *Service) activity(sessionID string) (SessionActivity, error) {
	states, err := s.runner.RunStates(sessionID)
	if err != nil {
		return SessionActivity{}, err
	}
	read, err := s.reading.Position(sessionID)
	if err != nil {
		return SessionActivity{}, err
	}
	activity := SessionActivity{SessionID: sessionID, ReadResultSeq: read}
	for _, state := range states {
		if state.Status == runner.RunRunning {
			activity.Running = true
			continue
		}
		if state.AfterEntrySeq > activity.LatestResultSeq {
			activity.LatestRunID = state.RunID
			activity.LatestResultSeq = state.AfterEntrySeq
		}
	}
	return activity, nil
}

// MarkRead 仅接受客户端实际看到的已结束 Run；不自动推进到后台最新结果。
func (s *Service) MarkRead(sessionID, runID string) error {
	operation, err := s.operation(sessionID)
	if err != nil {
		return err
	}
	operation.Lock()
	defer operation.Unlock()
	if _, err = s.Session(sessionID); err != nil {
		return err
	}
	states, err := s.runner.RunStates(sessionID)
	if err != nil {
		return err
	}
	for _, state := range states {
		if state.RunID != runID {
			continue
		}
		if state.Status == runner.RunRunning || state.AfterEntrySeq == 0 {
			break
		}
		return s.reading.Advance(sessionID, state.AfterEntrySeq)
	}
	return fmt.Errorf("%w: result is not complete", ErrRunChanged)
}
