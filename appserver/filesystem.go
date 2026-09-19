package appserver

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"harness/appserver/internal/clientconn"
	"harness/kernel/machine"
)

const maxEditorFileBytes = 2 << 20

const (
	readFileMethod      = "fs/readFile"
	writeFileMethod     = "fs/writeFile"
	readDirectoryMethod = "fs/readDirectory"
	searchPathsMethod   = "fs/searchPaths"
	getMetadataMethod   = "fs/getMetadata"
	watchMethod         = "fs/watch"
	changedMethod       = "fs/changed"
)

// 数据。一个绝对文件系统路径。
type FilePathParams struct {
	Path string `json:"path" jsonschema:"minLength=1"`
}

// 数据。读取到的文件字节及其 SHA-256 版本。
type ReadFileResult struct {
	DataBase64 string `json:"dataBase64"`
	Hash       string `json:"hash" jsonschema:"pattern=^[a-f0-9]{64}$"`
}

// 数据。只覆盖已读取版本的现存普通文件。
type WriteFileParams struct {
	Path         string `json:"path" jsonschema:"minLength=1"`
	DataBase64   string `json:"dataBase64"`
	ExpectedHash string `json:"expectedHash" jsonschema:"pattern=^[a-f0-9]{64}$"`
}

// 数据。保存成功后的新文件版本。
type WriteFileResult struct {
	Hash string `json:"hash" jsonschema:"pattern=^[a-f0-9]{64}$"`
}

// 数据。目录中的一个直接子项。
type FileEntry struct {
	FileName    string `json:"fileName" jsonschema:"minLength=1"`
	IsDirectory bool   `json:"isDirectory"`
	IsFile      bool   `json:"isFile"`
}

// 数据。目录的直接子项列表。
type ReadDirectoryResult struct {
	Entries []FileEntry `json:"entries"`
}

// 数据。在一个工作区内按名称或部分路径搜索；空查询只列根目录。
type SearchPathsParams struct {
	Workspace string `json:"workspace" jsonschema:"minLength=1"`
	Query     string `json:"query" jsonschema:"maxLength=1024"`
}

// 数据。文件或目录的类型与毫秒时间戳。
type GetMetadataResult struct {
	IsDirectory  bool  `json:"isDirectory"`
	IsFile       bool  `json:"isFile"`
	IsSymlink    bool  `json:"isSymlink"`
	CreatedAtMS  int64 `json:"createdAtMs"`
	ModifiedAtMS int64 `json:"modifiedAtMs"`
}

// 数据。文件监听成功后返回统一订阅身份。
type WatchResult struct {
	SubscriptionID string `json:"subscriptionID" jsonschema:"minLength=1"`
}

// 数据。文件监听只推送发生变化的路径。
type FileChangedEvent struct {
	ChangedPaths []string `json:"changedPaths"`
}

// BindFilesystem 显式接入公共 machine 服务，供富 Client 读取、保存和监听文件。
func (s *Server) BindFilesystem(filesystem machine.FileSystem) error {
	if filesystem == nil || s.filesystem != nil {
		return fmt.Errorf("appserver: nil or already bound filesystem")
	}
	pathSearcher, ok := filesystem.(machine.PathSearcher)
	if !ok {
		return fmt.Errorf("appserver: filesystem does not support path search")
	}
	s.filesystem = filesystem
	s.pathSearcher = pathSearcher
	if err := Register(s, readFileMethod, s.handleReadFile); err != nil {
		return err
	}
	if err := Register(s, writeFileMethod, s.handleWriteFile); err != nil {
		return err
	}
	if err := Register(s, readDirectoryMethod, s.handleReadDirectory); err != nil {
		return err
	}
	err := Register(s, searchPathsMethod, s.handleSearchPaths)
	if err != nil {
		return err
	}
	if err := Register(s, getMetadataMethod, s.handleGetMetadata); err != nil {
		return err
	}
	return Register(s, watchMethod, s.handleWatch)
}

func (s *Server) handleReadFile(_ context.Context, input FilePathParams) (ReadFileResult, error) {
	if err := checkAbsolutePath(input.Path); err != nil {
		return ReadFileResult{}, err
	}
	content, err := s.filesystem.ReadFileVersion(input.Path, maxEditorFileBytes)
	if err != nil {
		return ReadFileResult{}, filesystemMethodError(err)
	}
	return ReadFileResult{
		DataBase64: base64.StdEncoding.EncodeToString(content.Data),
		Hash:       content.Hash,
	}, nil
}

func (s *Server) handleWriteFile(_ context.Context, input WriteFileParams) (WriteFileResult, error) {
	if err := checkAbsolutePath(input.Path); err != nil {
		return WriteFileResult{}, err
	}
	data, err := base64.StdEncoding.DecodeString(input.DataBase64)
	if err != nil {
		return WriteFileResult{}, &Error{Code: CodeInvalidParams, Message: "file data is not valid base64", Cause: err}
	}
	if len(data) > maxEditorFileBytes {
		return WriteFileResult{}, &Error{Code: CodeInvalidParams, Message: "file exceeds 2 MiB limit", Cause: machine.ErrFileTooLarge}
	}
	hash, err := s.filesystem.WriteFileIfUnchanged(input.Path, data, input.ExpectedHash)
	if err != nil {
		return WriteFileResult{}, filesystemMethodError(err)
	}
	return WriteFileResult{Hash: hash}, nil
}

func (s *Server) handleReadDirectory(_ context.Context, input FilePathParams) (ReadDirectoryResult, error) {
	if err := checkAbsolutePath(input.Path); err != nil {
		return ReadDirectoryResult{}, err
	}
	entries, err := s.filesystem.ReadDir(input.Path)
	if err != nil {
		return ReadDirectoryResult{}, filesystemMethodError(err)
	}
	result := ReadDirectoryResult{Entries: make([]FileEntry, 0, len(entries))}
	for _, entry := range entries {
		result.Entries = append(result.Entries, FileEntry{
			FileName:    entry.Name,
			IsDirectory: entry.IsDir,
			IsFile:      entry.IsFile,
		})
	}
	return result, nil
}

func (s *Server) handleSearchPaths(ctx context.Context, input SearchPathsParams) (machine.PathSearchResult, error) {
	err := checkAbsolutePath(input.Workspace)
	if err != nil {
		return machine.PathSearchResult{}, err
	}
	result, err := s.pathSearcher.SearchPaths(ctx, input.Workspace, input.Query)
	return result, filesystemMethodError(err)
}

func (s *Server) handleGetMetadata(_ context.Context, input FilePathParams) (GetMetadataResult, error) {
	if err := checkAbsolutePath(input.Path); err != nil {
		return GetMetadataResult{}, err
	}
	metadata, err := s.filesystem.Metadata(input.Path)
	if err != nil {
		return GetMetadataResult{}, filesystemMethodError(err)
	}
	return GetMetadataResult{
		IsDirectory:  metadata.IsDir,
		IsFile:       metadata.IsFile,
		IsSymlink:    metadata.IsSymlink,
		CreatedAtMS:  metadata.CreatedAtMS,
		ModifiedAtMS: metadata.ModifiedAtMS,
	}, nil
}

func (s *Server) handleWatch(ctx context.Context, input FilePathParams) (WatchResult, error) {
	if err := checkAbsolutePath(input.Path); err != nil {
		return WatchResult{}, err
	}
	request, err := clientconn.FromContext(ctx)
	if err != nil {
		return WatchResult{}, err
	}
	subscription, err := request.Subscribe()
	if err != nil {
		return WatchResult{}, err
	}
	watch, err := s.filesystem.Watch(input.Path)
	if err != nil {
		subscription.Close()
		return WatchResult{}, filesystemMethodError(err)
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	subscription.SetCleanup(func() {
		close(stop)
		_ = watch.Close()
		<-done
	})
	go func() {
		defer close(done)
		defer watch.Close()
		for {
			select {
			case <-stop:
				return
			case event, ok := <-watch.Events():
				if !ok {
					select {
					case <-stop:
					default:
						subscription.Disconnect()
					}
					return
				}
				if event.Error != nil {
					subscription.Disconnect()
					return
				}
				if len(event.ChangedPaths) > 0 {
					subscription.Notify(changedMethod, FileChangedEvent{ChangedPaths: event.ChangedPaths})
				}
			}
		}
	}()
	return WatchResult{SubscriptionID: subscription.ID()}, nil
}

func checkAbsolutePath(path string) error {
	if !filepath.IsAbs(path) {
		return &Error{Code: CodeInvalidParams, Message: "absolute path required"}
	}
	return nil
}

func filesystemMethodError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return &Error{Code: CodeNotFound, Message: "path not found", Cause: err}
	}
	if errors.Is(err, machine.ErrFileConflict) {
		return &Error{Code: CodeConflict, Message: "file changed since it was read", Cause: err}
	}
	if errors.Is(err, machine.ErrFileTooLarge) {
		return &Error{Code: CodeInvalidParams, Message: "file exceeds 2 MiB limit", Cause: err}
	}
	if errors.Is(err, machine.ErrNotRegularFile) {
		return &Error{Code: CodeInvalidParams, Message: "path is not a regular file", Cause: err}
	}
	return err
}
