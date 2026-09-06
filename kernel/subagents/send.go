package subagents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"harness/kernel/session"
)

// Send 向指定孩子追加要求：空闲时开启新轮次并先存意图，忙时 Steer 且报错时不盲目重发。
func (s *Subagents) Send(ctx context.Context, parentSessionID, taskID string, input session.UserMessage) (SendResult, error) {
	err := ctx.Err()
	if err != nil {
		return SendResult{}, err
	}
	if taskID == "" {
		return SendResult{}, fmt.Errorf("subagents: empty task id")
	}
	var text strings.Builder
	for _, block := range input.Blocks {
		if block.Kind != "text" || block.Tool != nil || block.Result != nil || block.Media != nil {
			return SendResult{}, fmt.Errorf("subagents: send accepts only text")
		}
		text.WriteString(block.Text)
	}
	if strings.TrimSpace(text.String()) == "" {
		return SendResult{}, ErrDescriptionEmpty
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return SendResult{}, ErrClosed
	}
	s.inFlight.Add(1)
	coord := s.coords[taskID]
	s.mu.RUnlock()
	defer s.inFlight.Done()

	if coord == nil {
		return SendResult{}, ErrTaskNotFound
	}

	coord.mu.Lock()
	defer coord.mu.Unlock()

	for {
		if s.ctx.Err() != nil {
			return SendResult{}, ErrClosed
		}
		if coord.task.ParentSessionID != parentSessionID {
			return SendResult{}, ErrOwnershipMismatch
		}

		// 检查是否有活跃的 handle
		if coord.activeHandle != nil {
			select {
			case <-coord.activeHandle.Done():
				// 旧 handle 已经结束运行，但后台 onRunFinished 可能仍在进行保存/收尾！
				finCh := coord.finalizingCh
				if finCh != nil {
					select {
					case <-finCh:
						// 旧轮次已完成收尾
						coord.activeHandle = nil
					default:
						// 旧轮次尚未完成收尾：释放锁等待收尾，绝不能锁内等待导致死锁！
						coord.mu.Unlock()
						select {
						case <-finCh:
						case <-s.ctx.Done():
							coord.mu.Lock()
							return SendResult{}, ErrClosed
						case <-ctx.Done():
							coord.mu.Lock()
							return SendResult{}, ctx.Err()
						}
						coord.mu.Lock()
						// 重新检查关闭状态
						s.mu.RLock()
						closed := s.closed
						s.mu.RUnlock()
						if closed {
							return SendResult{}, ErrClosed
						}
						continue
					}
				} else {
					coord.activeHandle = nil
				}
			default:
				// 孩子正在运行中：通过 Steer 追加
				err := s.runner.Steer(coord.task.ChildSessionID, input)
				if err != nil {
					// Steer 落账或事件发布失败可能已经接收，不能盲重发，明确返回错误
					return SendResult{}, fmt.Errorf("subagents: steer child run: %w", err)
				}
				return SendResult{
					Turn:    coord.task.Turn,
					RunID:   coord.activeHandle.RunID(),
					Steered: true,
				}, nil
			}
		}
		break
	}

	// 检查资源完整性：创建失败导致资源不全时明确报错
	if coord.task.ChildSessionID == "" {
		return SendResult{}, ErrTaskNotUsable
	}
	_, err = s.sessions.Get(coord.task.ChildSessionID)
	if err != nil {
		return SendResult{}, fmt.Errorf("%w: %v", ErrTaskNotUsable, err)
	}
	_, err = s.settings.For(coord.task.ChildSessionID)
	if err != nil {
		return SendResult{}, fmt.Errorf("%w: %v", ErrTaskNotUsable, err)
	}

	// 若此前持久化失败，阻止盲目开启新轮
	if coord.persistErr != nil {
		return SendResult{}, fmt.Errorf("subagents: task %q in unpersisted error state: %w", taskID, coord.persistErr)
	}

	// 孩子当前处于空闲状态（正常完成 / 运行失败 / 取消 / 重启中断）：开启新轮次
	newTurn := coord.task.Turn + 1

	// 新轮次先保存 starting/pending 意图（保证重启知道未完成）
	coord.task.Turn = newTurn
	coord.task.Status = StatusPending
	coord.task.CurrentRunID = ""
	coord.task.ResultEntryID = ""
	coord.task.Error = ""
	now := time.Now().UTC()
	coord.task.UpdatedAt = now

	turnRec := TurnRecord{
		Turn:      newTurn,
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	coord.task.Turns = append(coord.task.Turns, turnRec)

	taskToSave := deepCopyTask(coord.task)
	coord.waitCh = make(chan struct{})
	coord.finalizingCh = make(chan struct{})
	coord.persistErr = nil

	err = s.store.saveTask(taskToSave)
	if err != nil {
		return SendResult{}, s.failTurn(coord, fmt.Errorf("%w: save pending turn: %w", ErrPersistFailed, err))
	}
	runID, err := s.startTurn(coord, input)
	if err != nil {
		return SendResult{}, err
	}
	return SendResult{Turn: newTurn, RunID: runID}, nil
}
