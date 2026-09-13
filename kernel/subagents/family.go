package subagents

import (
	"harness/kernel/runner"
)

// 数据。进程内的父会话停止边界；代次使停止前已接收的旧操作失效。
type familyState struct {
	generation   uint64
	stoppedRunID string
}

// 数据。一项已接收委派操作所属的父 Run 和停止代次，不使用协作级 Context。
type admission struct {
	parentSessionID string
	parentRunID     string
	generation      uint64
}

func (s *Subagents) admit(parentSessionID, parentRunID string) (admission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return admission{}, ErrClosed
	}
	if parentSessionID == "" || parentRunID == "" {
		return admission{}, ErrParentRequired
	}
	family := s.families[parentSessionID]
	if family.stoppedRunID == parentRunID {
		return admission{}, ErrFamilyStopped
	}
	state, active := s.runner.State(parentSessionID)
	if !active || state.RunID != parentRunID {
		return admission{}, ErrParentRequired
	}
	return admission{parentSessionID: parentSessionID, parentRunID: parentRunID, generation: family.generation}, nil
}

// admissionErrorLocked 的调用方持有 s.mu；父正常完成不使已接收操作失效。
func (s *Subagents) admissionErrorLocked(permit admission) error {
	if s.closed {
		return ErrClosed
	}
	family := s.families[permit.parentSessionID]
	if family.generation != permit.generation || (permit.parentRunID != "" && family.stoppedRunID == permit.parentRunID) {
		return ErrFamilyStopped
	}
	return nil
}

func (s *Subagents) childStartError(coord *taskCoord) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	err := s.admissionErrorLocked(coord.admission)
	if err != nil {
		return err
	}
	if coord.stopRequested {
		return ErrTaskStopped
	}
	return nil
}

// onRunStarted 是最后一道启动检查，覆盖业务预检通过后、Runner 登记前发生的停止。
// 不读取子 Session 投影，不等待 coord.mu，也不主动启动任何 Run。
func (s *Subagents) onRunStarted(event runner.RunEvent) error {
	s.mu.Lock()
	if taskID, child := s.childSessions[event.SessionID]; child {
		coord := s.coords[taskID]
		if coord.stopRequested || s.admissionErrorLocked(coord.admission) != nil {
			s.runner.StopRun(event.SessionID, event.RunID)
		}
		s.mu.Unlock()
		return nil
	}
	stopped := s.families[event.SessionID].stoppedRunID == event.RunID
	if stopped {
		s.runner.StopRun(event.SessionID, event.RunID)
	}
	s.mu.Unlock()
	if stopped {
		return nil
	}
	return s.deliver(event.SessionID)
}
