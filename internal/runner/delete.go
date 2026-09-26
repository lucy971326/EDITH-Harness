package runner

// ForgetSessions 丢弃已永久删除会话的进程内投影缓存；调用方先确认没有活 Run。
func (r *Runner) ForgetSessions(ids []string) {
	r.recordsMu.Lock()
	defer r.recordsMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		delete(r.clocks, id)
		delete(r.reconciled, id)
	}
}
