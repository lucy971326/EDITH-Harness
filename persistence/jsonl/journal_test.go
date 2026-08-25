package jsonl

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"harness/session"
)

func TestJSONLJournalCreatesWritesAndReads(t *testing.T) {
	journal, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	header := session.Header{FormatVersion: 1, ID: "测试账"}
	err = journal.Create(header.ID, header)
	if err != nil {
		t.Fatal(err)
	}
	assertPrivate(t, journal.root, 0o700)
	assertPrivate(t, journal.bookPath(header.ID), 0o600)
	first := testEvent(1, "你好")
	second := testEvent(2, "世界")
	err = journal.Append(header.ID, first, false)
	if err != nil {
		t.Fatal(err)
	}
	err = journal.Append(header.ID, second, true)
	if err != nil {
		t.Fatal(err)
	}

	gotHeader, events, err := journal.ReadAll(header.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotHeader, header) {
		t.Fatalf("封面不对：got %+v want %+v", gotHeader, header)
	}
	if !reflect.DeepEqual(events, []session.Event{first, second}) {
		t.Fatalf("事件不对：got %+v", events)
	}
}

func TestJournalReopensExistingBook(t *testing.T) {
	root := t.TempDir()
	first, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	header := session.Header{FormatVersion: 1, ID: "重启账"}
	err = first.Create(header.ID, header)
	if err != nil {
		t.Fatal(err)
	}
	err = first.Append(header.ID, testEvent(1, "重启前"), true)
	if err != nil {
		t.Fatal(err)
	}

	second, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	_, events, err := second.ReadAll(header.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || string(events[0].Data) != `{"text":"重启前"}` {
		t.Fatalf("重启后没有读回旧账：%+v", events)
	}
}

func TestListHeadersReadsOnlyBookCovers(t *testing.T) {
	journal, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"会话二", "会话一"} {
		header := session.Header{FormatVersion: 3, ID: id, ProjectID: "项目", PresetID: "模式", PresetRevision: 1}
		err = journal.Create(id, header)
		if err != nil {
			t.Fatal(err)
		}
	}
	headers, err := journal.ListHeaders()
	if err != nil {
		t.Fatal(err)
	}
	if len(headers) != 2 || headers[0].ID != "会话一" || headers[1].ID != "会话二" {
		t.Fatalf("封面应按账本号排序：%+v", headers)
	}
}

func TestFlushWritesBufferedEvents(t *testing.T) {
	journal, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	header := session.Header{FormatVersion: 1, ID: "缓冲账"}
	err = journal.Create(header.ID, header)
	if err != nil {
		t.Fatal(err)
	}
	err = journal.Append(header.ID, testEvent(1, "先攒着"), false)
	if err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadFile(journal.bookPath(header.ID))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(before, []byte{'\n'}) != 1 {
		t.Fatalf("Flush 前文件里应该只有封面：%q", before)
	}
	err = journal.Flush(header.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(journal.bookPath(header.ID))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(after, []byte{'\n'}) != 2 {
		t.Fatalf("Flush 后事件应该落盘：%q", after)
	}
}

func TestAppendFailureRemovesPartialLine(t *testing.T) {
	journal, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	header := session.Header{FormatVersion: 1, ID: "失败账"}
	err = journal.Create(header.ID, header)
	if err != nil {
		t.Fatal(err)
	}

	journal.writeFile = func(file *os.File, data []byte) error {
		_, writeErr := file.Write(data[:len(data)/2])
		if writeErr != nil {
			return writeErr
		}
		return errors.New("模拟磁盘写到一半失败")
	}
	err = journal.Append(header.ID, testEvent(1, "不能留半条"), true)
	if err == nil {
		t.Fatal("写到一半应该失败")
	}
	journal.writeFile = writeFull

	data, err := os.ReadFile(journal.bookPath(header.ID))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(data, []byte{'\n'}) != 1 {
		t.Fatalf("失败后文件里只能有封面：%q", data)
	}
	err = journal.Append(header.ID, testEvent(1, "重新写"), true)
	if err != nil {
		t.Fatal(err)
	}
}

func TestReadRepairsIncompleteLastLine(t *testing.T) {
	journal, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	header := session.Header{FormatVersion: 1, ID: "烂尾账"}
	err = journal.Create(header.ID, header)
	if err != nil {
		t.Fatal(err)
	}
	err = journal.Append(header.ID, testEvent(1, "完整"), true)
	if err != nil {
		t.Fatal(err)
	}
	appendRaw(t, journal.bookPath(header.ID), []byte(`{"Kind":"user/message"`))

	_, events, err := journal.ReadAll(header.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("只该丢掉最后半条：%+v", events)
	}
	data, err := os.ReadFile(journal.bookPath(header.ID))
	if err != nil {
		t.Fatal(err)
	}
	if data[len(data)-1] != '\n' {
		t.Fatal("修完后文件应该停在上一条完整记录后")
	}
}

func TestReadRejectsBrokenCompleteLine(t *testing.T) {
	journal, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	header := session.Header{FormatVersion: 1, ID: "中段坏账"}
	err = journal.Create(header.ID, header)
	if err != nil {
		t.Fatal(err)
	}
	appendRaw(t, journal.bookPath(header.ID), []byte("{坏掉了}\n"))

	_, _, err = journal.ReadAll(header.ID)
	if err == nil {
		t.Fatal("有换行的坏记录不能假装成烂尾删掉")
	}
}

func TestReadRejectsBrokenSequence(t *testing.T) {
	journal, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	header := session.Header{FormatVersion: 1, ID: "断号账"}
	err = journal.Create(header.ID, header)
	if err != nil {
		t.Fatal(err)
	}
	line, err := encodeLine(testEvent(2, "跳号"))
	if err != nil {
		t.Fatal(err)
	}
	appendRaw(t, journal.bookPath(header.ID), line)

	_, _, err = journal.ReadAll(header.ID)
	if err == nil {
		t.Fatal("编号不连续必须拒绝读账")
	}
}

func assertPrivate(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("%s 权限是 %o，想要 %o", path, info.Mode().Perm(), want)
	}
}

func testEvent(seq int, text string) session.Event {
	data, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		panic(err)
	}
	return session.Event{
		Kind: session.KindUserMessage,
		Seq:  seq,
		Time: int64(seq),
		Data: data,
	}
}

func appendRaw(t *testing.T, path string, data []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Write(data)
	if err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}
}
