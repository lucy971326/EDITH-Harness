// Package jsonl 提供把 session 账本保存为 JSONL 文件的持久化插件。
package jsonl

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"harness/session"
)

// Journal 把每本账保存成单独的 JSONL 文件。
type Journal struct {
	mu        sync.Mutex                   // 同一时刻只读写一本文件
	root      string                       // 所有账本文件所在目录
	pending   map[string][]byte            // 还没要求立刻写盘的流式事件
	writeFile func(*os.File, []byte) error // 写入函数；测试用它模拟短写
}

var _ session.Journal = (*Journal)(nil)

// New 建立文件账本目录；账本文件在 Create 时自动生成。
func New(root string) (*Journal, error) {
	if root == "" {
		return nil, errors.New("JSONL 账本目录不能为空")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("解析 JSONL 账本目录失败：%w", err)
	}
	err = os.MkdirAll(absolute, 0o755)
	if err != nil {
		return nil, fmt.Errorf("创建 JSONL 账本目录失败：%w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("打开 JSONL 账本目录失败：%w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("JSONL 账本路径 %s 不是目录", absolute)
	}
	return &Journal{
		root:      absolute,
		pending:   make(map[string][]byte),
		writeFile: writeFull,
	}, nil
}

// Create 新建一本账并把封面写到硬盘；同名账本不会被覆盖。
func (j *Journal) Create(id string, header session.Header) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if header.ID != id {
		return fmt.Errorf("账本号 %s 与封面里的 %s 不一致", id, header.ID)
	}
	line, err := encodeLine(header)
	if err != nil {
		return fmt.Errorf("账本 %s 的封面无法编码：%w", id, err)
	}
	path := j.bookPath(id)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("创建账本 %s 失败：%w", id, err)
	}

	err = j.writeFile(file, line)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("写账本 %s 的封面失败：%w", id, err)
	}
	return nil
}

// Append 写入一笔；durable=false 时先留在内存，等关键账或 Flush 一起落盘。
func (j *Journal) Append(id string, event session.Event, durable bool) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	err := j.requireBook(id)
	if err != nil {
		return err
	}
	line, err := encodeLine(event)
	if err != nil {
		return fmt.Errorf("账本 %s 的第 %d 笔无法编码：%w", id, event.Seq, err)
	}
	if !durable {
		j.pending[id] = append(j.pending[id], line...)
		return nil
	}

	batch := make([]byte, 0, len(j.pending[id])+len(line))
	batch = append(batch, j.pending[id]...)
	batch = append(batch, line...)
	err = j.commit(id, batch)
	if err != nil {
		return err
	}
	delete(j.pending, id)
	return nil
}

// Flush 把暂存在内存里的流式事件全部写完并同步到硬盘。
func (j *Journal) Flush(id string) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	err := j.requireBook(id)
	if err != nil {
		return err
	}
	err = j.commit(id, j.pending[id])
	if err != nil {
		return err
	}
	delete(j.pending, id)
	return nil
}

// ReadAll 读回一本账；只修掉没有换行的最后半条，中间损坏直接报错。
func (j *Journal) ReadAll(id string) (session.Header, []session.Event, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	err := j.requireBook(id)
	if err != nil {
		return session.Header{}, nil, err
	}
	err = j.commit(id, j.pending[id])
	if err != nil {
		return session.Header{}, nil, err
	}
	delete(j.pending, id)

	path := j.bookPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		return session.Header{}, nil, fmt.Errorf("读取账本 %s 失败：%w", id, err)
	}
	data, err = repairTail(path, data)
	if err != nil {
		return session.Header{}, nil, fmt.Errorf("修复账本 %s 的最后半条失败：%w", id, err)
	}
	lines := bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'})
	if len(lines) == 0 || len(lines[0]) == 0 {
		return session.Header{}, nil, fmt.Errorf("账本 %s 没有完整封面", id)
	}

	var header session.Header
	err = json.Unmarshal(lines[0], &header)
	if err != nil {
		return session.Header{}, nil, fmt.Errorf("账本 %s 的封面坏了：%w", id, err)
	}
	if header.ID != id {
		return session.Header{}, nil, fmt.Errorf("账本文件属于 %s，不是 %s", header.ID, id)
	}

	events := make([]session.Event, 0, len(lines)-1)
	for index, line := range lines[1:] {
		var event session.Event
		err = json.Unmarshal(line, &event)
		if err != nil {
			return session.Header{}, nil, fmt.Errorf("账本 %s 的第 %d 行坏了：%w", id, index+2, err)
		}
		wantSeq := index + 1
		if event.Seq != wantSeq {
			return session.Header{}, nil, fmt.Errorf("账本 %s 编号断了：第 %d 笔的编号是 %d", id, wantSeq, event.Seq)
		}
		events = append(events, event)
	}
	return header, events, nil
}

// commit 把一批完整行追加到文件；任何一步失败都退回追加前的位置。
func (j *Journal) commit(id string, data []byte) error {
	path := j.bookPath(id)
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("打开账本 %s 失败：%w", id, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("读取账本 %s 大小失败：%w", id, err)
	}
	start := info.Size()
	_, err = file.Seek(start, io.SeekStart)
	if err != nil {
		return fmt.Errorf("定位账本 %s 末尾失败：%w", id, err)
	}
	if len(data) > 0 {
		err = j.writeFile(file, data)
		if err != nil {
			return rollback(file, start, fmt.Errorf("追加账本 %s 失败：%w", id, err))
		}
	}
	err = file.Sync()
	if err != nil {
		return rollback(file, start, fmt.Errorf("同步账本 %s 失败：%w", id, err))
	}
	return nil
}

func (j *Journal) requireBook(id string) error {
	_, err := os.Stat(j.bookPath(id))
	if err != nil {
		return fmt.Errorf("账本 %s 不存在：%w", id, err)
	}
	return nil
}

func (j *Journal) bookPath(id string) string {
	name := base64.RawURLEncoding.EncodeToString([]byte(id))
	if name == "" {
		name = "empty"
	}
	return filepath.Join(j.root, name+".jsonl")
}

func encodeLine(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func writeFull(file *os.File, data []byte) error {
	for len(data) > 0 {
		written, err := file.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

func rollback(file *os.File, offset int64, cause error) error {
	truncateErr := file.Truncate(offset)
	syncErr := file.Sync()
	if truncateErr == nil && syncErr == nil {
		return cause
	}
	return errors.Join(cause, truncateErr, syncErr)
}

func repairTail(path string, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}
	if data[len(data)-1] == '\n' {
		return data, nil
	}
	lastNewline := bytes.LastIndexByte(data, '\n')
	if lastNewline < 0 {
		return nil, errors.New("连封面都没有写完整")
	}
	keep := int64(lastNewline + 1)
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	err = file.Truncate(keep)
	if err != nil {
		return nil, err
	}
	err = file.Sync()
	if err != nil {
		return nil, err
	}
	return data[:keep], nil
}
