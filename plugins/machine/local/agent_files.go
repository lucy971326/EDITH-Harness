package machinelocal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"harness/kernel/machine"
	"harness/kernel/permissions"
)

const fileWorkerArgument = "--internal-apply-changes"

type fileWorkerResult struct {
	machine.FileCommit
	Error string
}

// RunFileWorker 供可执行文件入口分流；不创建 Host，不启动其他服务。
func RunFileWorker(args []string, input io.Reader, output io.Writer) (bool, error) {
	if len(args) == 0 || args[0] != fileWorkerArgument {
		return false, nil
	}
	if len(args) != 1 {
		return true, fmt.Errorf("machine: unexpected file worker arguments")
	}
	var changes []machine.FileChange
	err := json.NewDecoder(input).Decode(&changes)
	if err != nil {
		return true, err
	}
	encoder := json.NewEncoder(output)
	result, commitErr := commitFileChanges(context.Background(), changes, func(result machine.FileCommit) error {
		return encoder.Encode(fileWorkerResult{FileCommit: result})
	})
	reply := fileWorkerResult{FileCommit: result}
	if commitErr != nil {
		reply.Error = commitErr.Error()
	}
	return true, encoder.Encode(reply)
}

func (m *local) AgentApplyChanges(ctx context.Context, policy permissions.Policy, changes []machine.FileChange) (machine.FileCommit, error) {
	empty := machine.FileCommit{Exact: true}
	paths := make([]string, 0, len(changes))
	for _, change := range changes {
		paths = append(paths, change.Path)
	}
	requirement, err := permissions.Evaluate(policy, permissions.ExtraPermissions{WriteRoots: paths})
	if err != nil {
		return empty, err
	}
	if requirement != permissions.Allow {
		return empty, fmt.Errorf("machine: file changes require additional permission; approval is not connected yet")
	}
	if len(changes) == 0 {
		return empty, nil
	}
	// 与编辑器使用相同的锁；排序并去重，避免多文件请求互相死锁。
	keys := make([]string, 0, len(paths))
	for _, path := range paths {
		keys = append(keys, canonicalPath(path))
	}
	sort.Strings(keys)
	var unlocks []func()
	defer func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}()
	for i, key := range keys {
		if i > 0 && keys[i-1] == key {
			return empty, fmt.Errorf("machine: multiple changes target the same file")
		}
		unlock, lockErr := m.fileLocks.lockKey(ctx, key)
		if lockErr != nil {
			return empty, lockErr
		}
		unlocks = append(unlocks, unlock)
	}
	err = ctx.Err()
	if err != nil {
		return empty, err
	}

	// Full Access 也用可终止的助手，只省略沙箱，避免文件 I/O 占用宿主服务锁。

	process, processID, err := m.startFileWorker(ctx, policy, changes)
	if err != nil {
		return empty, err
	}
	defer func() {
		if process.hasExited() {
			m.removeProcess(processID, process)
		}
	}()
	select {
	case <-process.done:
	case <-ctx.Done():
		_ = process.terminate()
		_ = process.waitUntil(time.Now().Add(processTerminationTimeout))
	}
	process.mu.Lock()
	data, omitted := process.output.take()
	exited, exitCode := process.exited, process.exitCode
	process.mu.Unlock()
	result := machine.FileCommit{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var reply fileWorkerResult
		decodeErr := decoder.Decode(&reply)
		if errors.Is(decodeErr, io.EOF) {
			break
		}
		if decodeErr != nil {
			return machine.FileCommit{Completed: result.Completed}, fmt.Errorf("machine: incomplete file worker response: %s", data)
		}
		if reply.Completed < result.Completed || reply.Completed > len(changes) {
			return machine.FileCommit{}, fmt.Errorf("machine: invalid file worker progress")
		}
		result = reply.FileCommit
		if reply.Error != "" {
			err = errors.New(reply.Error)
		}
	}
	if ctx.Err() != nil || !exited || exitCode != 0 || omitted != 0 || len(data) == 0 {
		result.Exact = false
		return result, fmt.Errorf("machine: file worker interrupted or failed (exit %d): %w", exitCode, errors.Join(ctx.Err(), err, errors.New(string(data))))
	}
	return result, err
}

// 启动、登记与关闭检查在同一把锁内；等待与读取不占用服务锁。
func (m *local) startFileWorker(ctx context.Context, policy permissions.Policy, changes []machine.FileChange) (*localProcess, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, 0, fmt.Errorf("machine: machine is closed")
	}
	if len(m.processes) >= maxTrackedProcesses {
		m.pruneExitedProcessLocked()
	}
	if len(m.processes) >= maxTrackedProcesses {
		return nil, 0, fmt.Errorf("machine: process limit reached")
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, 0, err
	}
	payload, err := json.Marshal(changes)
	if err != nil {
		return nil, 0, err
	}
	request := machine.ProcessRequest{Argv: []string{executable, fileWorkerArgument}}
	launch, err := m.prepareAgentLaunch(policy, request)
	if err != nil {
		return nil, 0, err
	}
	launch.cmd.Stdin = bytes.NewReader(payload)
	processID, err := m.newProcessIDLocked()
	if err != nil {
		launch.close()
		return nil, 0, err
	}
	err = ctx.Err()
	if err != nil {
		launch.close()
		return nil, 0, err
	}
	process, err := m.startPreparedProcess(request, launch.cmd, 80, 24, false, launch.close)
	if err != nil {
		launch.close()
		return nil, 0, err
	}
	m.processes[processID] = process
	return process, processID, nil
}

func commitFileChanges(ctx context.Context, changes []machine.FileChange, progress func(machine.FileCommit) error) (machine.FileCommit, error) {
	result := machine.FileCommit{Exact: true}
	// 独立的短命写入器；宿主在批次期间持有跨入口共享的路径锁。
	files := &local{}
	for _, change := range changes {
		err := ctx.Err()
		if err != nil {
			return result, err
		}
		if change.Content == nil {
			err = files.RemoveFileIfUnchanged(change.Path, change.ExpectedHash)
		} else {
			_, err = files.WriteFileIfUnchanged(change.Path, []byte(*change.Content), change.ExpectedHash)
		}
		if err != nil {
			current, readErr := files.ReadFileVersion(change.Path, 0)
			result.Exact = errors.Is(err, machine.ErrFileConflict) || (readErr == nil && current.Hash == change.ExpectedHash) || (os.IsNotExist(readErr) && change.ExpectedHash == machine.AbsentFileHash)
			return result, err
		}
		result.Completed++
		if progress != nil {
			err = progress(result)
			if err != nil {
				return result, err
			}
		}
	}
	return result, nil
}
