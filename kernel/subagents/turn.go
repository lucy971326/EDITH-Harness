package subagents

import (
	"errors"
	"fmt"
	"time"

	"harness/kernel/session"
)

// startTurn 在持有 coord.mu 时启动并保存本轮；调用方仍计入 inFlight。
func (s *Subagents) startTurn(coord *taskCoord, input session.UserMessage) (string, error) {
	if s.ctx.Err() != nil {
		return "", s.failTurn(coord, ErrClosed)
	}
	handle, err := s.runner.Start(s.ctx, coord.task.ChildSessionID, input)
	if err != nil {
		return "", s.failTurn(coord, fmt.Errorf("start child run: %w", err))
	}
	coord.activeHandle = handle
	coord.task.CurrentRunID = handle.RunID()
	coord.task.Status = StatusRunning
	coord.task.UpdatedAt = time.Now().UTC()
	rec := &coord.task.Turns[len(coord.task.Turns)-1]
	rec.RunID = handle.RunID()
	rec.Status = StatusRunning
	rec.UpdatedAt = coord.task.UpdatedAt

	// 成功启动就必须追踪。回调先等待 coord.mu，不能抢先写入完成状态。
	s.trackRun(coord.task.ID, coord.task.Turn, handle, coord)
	err = s.store.saveTask(coord.task)
	if err != nil {
		coord.persistErr = fmt.Errorf("%w: save running task: %w", ErrPersistFailed, err)
		_ = s.runner.Stop(coord.task.ChildSessionID)
		return "", coord.persistErr
	}
	if s.ctx.Err() != nil {
		// Start 使用服务 context，关闭已取消该 Run；仍等待正常回调收尾。
		return "", ErrClosed
	}
	return handle.RunID(), nil
}

// failTurn 收尾未能启动的轮次。调用方持有 coord.mu；错误与唤醒均不依赖后台回调。
func (s *Subagents) failTurn(coord *taskCoord, cause error) error {
	task := &coord.task
	task.Status = StatusFailed
	if errors.Is(cause, ErrClosed) {
		task.Status = StatusCancelled
	}
	task.Error = cause.Error()
	task.UpdatedAt = time.Now().UTC()
	rec := &task.Turns[len(task.Turns)-1]
	rec.Status = task.Status
	rec.Error = task.Error
	rec.UpdatedAt = task.UpdatedAt
	task.Notifications = append(task.Notifications, Notification{
		NotificationID: notificationID(task.ID, task.Turn),
		TaskID:         task.ID, ChildSessionID: task.ChildSessionID,
		Turn: task.Turn, Status: task.Status, Error: task.Error,
		CreatedAt: task.UpdatedAt,
	})
	if errors.Is(cause, ErrPersistFailed) {
		coord.persistErr = cause
	}
	err := s.store.saveTask(*task)
	if err != nil {
		coord.persistErr = errors.Join(coord.persistErr, fmt.Errorf("%w: %w", ErrPersistFailed, err))
	}
	close(coord.waitCh)
	close(coord.finalizingCh)
	return errors.Join(cause, coord.persistErr)
}

func notificationID(taskID string, turn int) string {
	return fmt.Sprintf("%s-turn-%d", taskID, turn)
}
