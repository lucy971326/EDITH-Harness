package subagents

import (
	"fmt"

	"harness/kernel/runner"
)

const maxDelegationDepth = 2

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

// sessionDepthLocked 返回指定 Session 在委派树中的深度；根会话为 0。
// 调用方持有 s.mu；TaskRecord 发布后不再修改。
func (s *Subagents) sessionDepthLocked(sessionID string) (int, error) {
	depth := 0
	seen := make(map[string]struct{})
	for {
		taskID, child := s.childSessions[sessionID]
		if !child {
			return depth, nil
		}
		if _, duplicate := seen[sessionID]; duplicate {
			return 0, fmt.Errorf("%w: task relation cycle at session %q", ErrInvalidTaskData, sessionID)
		}
		seen[sessionID] = struct{}{}
		coord := s.coords[taskID]
		if coord == nil {
			return 0, fmt.Errorf("%w: task %q missing from relation index", ErrInvalidTaskData, taskID)
		}
		depth++
		sessionID = coord.record.ParentSessionID
	}
}

// validateTaskGraphLocked 拒绝循环和超过当前实现上限的持久关系。
// 构造期间调用，此时关系索引尚未并发发布。
func (s *Subagents) validateTaskGraphLocked() error {
	for childSessionID := range s.childSessions {
		depth, err := s.sessionDepthLocked(childSessionID)
		if err != nil {
			return err
		}
		if depth > maxDelegationDepth {
			return fmt.Errorf("%w: session %q has depth %d, maximum is %d", ErrDepthLimit, childSessionID, depth, maxDelegationDepth)
		}
	}
	return nil
}

func (s *Subagents) admit(parentSessionID, parentRunID string) (admission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
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
	if s.ctx.Err() != nil {
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
		stopped := coord.stopRequested || s.admissionErrorLocked(coord.admission) != nil
		if stopped {
			s.runner.StopRun(event.SessionID, event.RunID)
		}
		s.mu.Unlock()
		if stopped {
			return nil
		}
		// 多层委派中，孩子也可能是父亲；在首次模型请求前投递它的孩子回报。
		return s.deliver(event.SessionID)
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
