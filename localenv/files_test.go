package localenv

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"harness/workspace/files"
)

func TestLocalStoreWritesReadsAndLists(t *testing.T) {
	store, err := newFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	err = store.Write(context.Background(), "notes/today.txt", []byte("今天开工"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := store.Read(context.Background(), "notes/today.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "今天开工" {
		t.Fatalf("读回的内容不对：%s", data)
	}

	entries, err := store.List(context.Background(), "notes")
	if err != nil {
		t.Fatal(err)
	}
	want := []files.Entry{{Name: "today.txt", Size: int64(len(data))}}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("列目录结果不对：got %+v, want %+v", entries, want)
	}
}

func TestLocalStoreRejectsPathsOutsideRoot(t *testing.T) {
	store, err := newFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Read(context.Background(), "../secret")
	if err == nil {
		t.Fatal("跨出根目录的路径必须被拒绝")
	}
	_, err = store.Read(context.Background(), filepath.Join(string(filepath.Separator), "secret"))
	if err == nil {
		t.Fatal("绝对路径必须被拒绝")
	}
}

func TestLocalStoreHonorsCancelledContext(t *testing.T) {
	store, err := newFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = store.Write(ctx, "ignored.txt", []byte("no"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("应返回 context.Canceled：got %v", err)
	}
}
