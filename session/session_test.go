package session

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// recordingBroadcaster 记下每次广播；钩子模拟"回调里干点什么"。
type recordingBroadcaster struct {
	mu      sync.Mutex
	events  []Event
	ids     []string
	onEvent func(Event)
}

func (b *recordingBroadcaster) Broadcast(name string, payload any) {
	if name != EventAppended {
		return
	}
	appended := payload.(Appended)
	event := appended.Event

	b.mu.Lock()
	b.events = append(b.events, event)
	b.ids = append(b.ids, appended.SessionID)
	hook := b.onEvent
	b.mu.Unlock()

	if hook != nil {
		hook(event)
	}
}

func (b *recordingBroadcaster) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

func (b *recordingBroadcaster) lastID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ids[len(b.ids)-1]
}

// failingJournal 可以注入落盘失败：磁盘满的替身。
type failingJournal struct {
	inner *MemoryJournal
	fail  bool
}

func validHeader(id string) Header {
	return Header{
		ID:             id,
		Title:          id,
		CreatedAt:      time.Unix(1, 0),
		ProjectID:      "测试项目",
		ProjectRoot:    "/tmp/测试项目",
		PresetID:       "测试模式",
		PresetRevision: 1,
	}
}

func (j *failingJournal) Create(id string, header Header) error {
	return j.inner.Create(id, header)
}

func (j *failingJournal) Append(id string, event Event, durable bool) error {
	if j.fail {
		return errors.New("磁盘满")
	}
	return j.inner.Append(id, event, durable)
}

func (j *failingJournal) Flush(id string) error {
	return j.inner.Flush(id)
}

func (j *failingJournal) ReadAll(id string) (Header, []Event, error) {
	return j.inner.ReadAll(id)
}

func (j *failingJournal) ListHeaders() ([]Header, error) {
	return j.inner.ListHeaders()
}

func newTestStore(t *testing.T) (*Store, *Session, *recordingBroadcaster) {
	t.Helper()

	broadcaster := &recordingBroadcaster{}
	store := NewStore(NewMemoryJournal(), broadcaster)

	s, err := store.Create(validHeader("测试账"))
	if err != nil {
		t.Fatalf("开账失败：%v", err)
	}
	return store, s, broadcaster
}

func TestAppendEventRejectsKernelKind(t *testing.T) {
	_, s, _ := newTestStore(t)

	_, err := s.AppendEvent(KindToolCall, ToolCallData{ID: "c1", Name: "bash"})
	if err == nil {
		t.Fatal("内核种类走公开口应该被拒绝——一家管一种账")
	}
}

func TestAppendEventRequiresRegisteredKind(t *testing.T) {
	_, s, _ := newTestStore(t)

	_, err := s.AppendEvent("todo/note", map[string]string{"text": "买菜"})
	if err == nil {
		t.Fatal("没注册过的种类不该能记账")
	}
}

func TestAppendRejectsUnserializableData(t *testing.T) {
	store, s, _ := newTestStore(t)

	err := store.RegisterKind("broken/stuff", false)
	if err != nil {
		t.Fatalf("注册失败：%v", err)
	}

	_, err = s.AppendEvent("broken/stuff", map[string]any{"ch": make(chan int)})
	if err == nil {
		t.Fatal("记不成 JSON 的内容应该当场报错")
	}
	if len(s.Events()) != 0 {
		t.Fatal("失败这笔不该入账")
	}
}

func TestReplacesMustTargetExistingSeq(t *testing.T) {
	_, s, _ := newTestStore(t)

	_, err := s.RecordSummary(SummaryData{Text: "摘要"}, []int{7})
	if err == nil {
		t.Fatal("要取代不存在的编号应该报错")
	}
}

func TestEveryAppendBroadcastsOnce(t *testing.T) {
	_, s, broadcaster := newTestStore(t)

	_, err := s.RecordUserMessage("你好")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}
	_, err = s.RecordChunk("回")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}

	if broadcaster.count() != 2 {
		t.Fatalf("记几笔记几次：got %d 次", broadcaster.count())
	}
	if broadcaster.lastID() != "测试账" {
		t.Fatalf("广播要报是哪本账：got %q", broadcaster.lastID())
	}
}

func TestReadDoesNotOpenBook(t *testing.T) {
	store, book, _ := newTestStore(t)
	_, err := book.RecordUserMessage("只读历史")
	if err != nil {
		t.Fatal(err)
	}
	err = store.Release(book.ID())
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.Read(book.ID())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Get(book.ID())
	if err == nil {
		t.Fatal("只读历史不该把账本放进打开表")
	}
}

func TestFailedAppendLeavesNoTrace(t *testing.T) {
	broadcaster := &recordingBroadcaster{}
	journal := &failingJournal{inner: NewMemoryJournal()}
	store := NewStore(journal, broadcaster)

	s, err := store.Create(validHeader("会失败的账"))
	if err != nil {
		t.Fatalf("开账失败：%v", err)
	}

	_, err = s.RecordUserMessage("第一笔")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}

	journal.fail = true
	_, err = s.RecordUserMessage("注定失败的第二笔")
	if err == nil {
		t.Fatal("落盘失败应该报错")
	}
	journal.fail = false

	if len(s.Events()) != 1 {
		t.Fatalf("失败的笔不该进内存账：got %d 笔", len(s.Events()))
	}
	if broadcaster.count() != 1 {
		t.Fatalf("失败的笔不该广播：got %d 次", broadcaster.count())
	}

	event, err := s.RecordUserMessage("补上的第二笔")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}
	if event.Seq != 2 {
		t.Fatalf("失败后编号该被下一笔复用：got %d", event.Seq)
	}
}

func TestSyncAppendInBroadcastCallbackRejected(t *testing.T) {
	store, s, broadcaster := newTestStore(t)

	err := store.RegisterKind("todo/note", false)
	if err != nil {
		t.Fatalf("注册失败：%v", err)
	}

	var rejected error
	broadcaster.onEvent = func(Event) {
		_, rejected = s.AppendEvent("todo/note", map[string]string{"text": "回调里偷记一笔"})
	}

	_, err = s.RecordUserMessage("正经一笔")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}
	if !errors.Is(rejected, ErrInBroadcast) {
		t.Fatalf("回调里同步记账该被拒绝：got %v", rejected)
	}
	if len(s.Events()) != 1 {
		t.Fatalf("被拒的笔不该入账：got %d 笔", len(s.Events()))
	}
}

func TestKernelRecordsWaitForAnotherBroadcast(t *testing.T) {
	_, s, broadcaster := newTestStore(t)

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	broadcaster.onEvent = func(Event) {
		once.Do(func() {
			close(entered)
			<-release
		})
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := s.RecordDeliver("d1", "第一条", "next-turn")
		firstDone <- err
	}()
	<-entered

	secondDone := make(chan error, 1)
	go func() {
		_, err := s.RecordClaim("d1")
		secondDone <- err
	}()

	select {
	case err := <-secondDone:
		t.Fatalf("另一条内核记账应该等广播结束，不该抢跑或失败：%v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	err := <-firstDone
	if err != nil {
		t.Fatalf("第一笔失败：%v", err)
	}
	err = <-secondDone
	if err != nil {
		t.Fatalf("第二笔失败：%v", err)
	}

	events := s.Events()
	if len(events) != 2 || events[0].Seq != 1 || events[1].Seq != 2 {
		t.Fatalf("并发记账应该排好队：%+v", events)
	}
}

func TestAppendLaterDeferredUntilBroadcastEnds(t *testing.T) {
	store, s, broadcaster := newTestStore(t)

	err := store.RegisterKind("todo/note", false)
	if err != nil {
		t.Fatalf("注册失败：%v", err)
	}

	// 只投一次，否则钩子会在补记的广播里再投、没完没了。
	invested := false
	broadcaster.onEvent = func(Event) {
		if invested {
			return
		}
		invested = true
		err := s.AppendLater("todo/note", map[string]string{"text": "回调里投的账"})
		if err != nil {
			t.Errorf("AppendLater 不该被拒：%v", err)
		}
	}

	_, err = s.RecordUserMessage("正经一笔")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}

	if len(s.Events()) != 2 {
		t.Fatalf("广播结束后补记该入账：got %d 笔", len(s.Events()))
	}
	if broadcaster.count() != 2 {
		t.Fatalf("补记也该广播：got %d 次", broadcaster.count())
	}
	if s.Events()[1].Kind != "todo/note" {
		t.Fatalf("第二笔该是补记的：got %s", s.Events()[1].Kind)
	}
}

func TestEventsReturnsDeepCopy(t *testing.T) {
	_, s, _ := newTestStore(t)

	_, err := s.RecordUserMessage("你好")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}

	events := s.Events()
	events[0].Kind = "被篡改"
	events[0].Data = []byte(`{"被":"篡改"}`)
	events[0].Replaces = []int{99}

	fresh := s.Events()
	if fresh[0].Kind != KindUserMessage || len(fresh[0].Replaces) != 0 {
		t.Fatalf("改副本不该影响账：got %+v", fresh[0])
	}
}

func TestDataIsLockedAtAppend(t *testing.T) {
	store, s, _ := newTestStore(t)
	err := store.RegisterKind("todo/note", false)
	if err != nil {
		t.Fatalf("注册失败：%v", err)
	}

	draft := map[string]any{"text": "买菜"}
	_, err = s.AppendEvent("todo/note", draft)
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}

	draft["text"] = "改主意了"

	events := s.Events()
	if string(events[0].Data) != `{"text":"买菜"}` {
		t.Fatalf("记进去就该锁死：got %s", events[0].Data)
	}
}

func TestFlushWritesPendingBook(t *testing.T) {
	_, s, _ := newTestStore(t)

	_, err := s.RecordChunk("攒着的字")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}

	err = s.Flush()
	if err != nil {
		t.Fatalf("把攒着的写完不该失败：%v", err)
	}
}
