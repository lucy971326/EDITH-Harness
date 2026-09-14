// Package machine 定义当前机器服务的契约。
package machine

import (
	"context"
	"errors"
	"time"
)

// AbsentFileHash 表示调用方读取时文件不存在；SHA-256 不会产生空字符串。
const AbsentFileHash = ""

var (
	// ErrFileConflict 表示文件内容已不是调用方读取到的版本。
	ErrFileConflict = errors.New("machine: file changed")
	// ErrFileTooLarge 表示文件超过调用方允许读取的大小。
	ErrFileTooLarge = errors.New("machine: file too large")
	// ErrNotRegularFile 表示路径不是可读写的普通文件。
	ErrNotRegularFile = errors.New("machine: not a regular file")
)

// 数据。当前机器目录中的一个直接子项。
type DirEntry struct {
	Name   string
	IsDir  bool
	IsFile bool
}

// 数据。一份文件内容及其 SHA-256 版本。
type FileContent struct {
	Data []byte
	Hash string
}

// 数据。文件或目录的类型与时间信息；系统无法提供的创建时间为 0。
type FileMetadata struct {
	IsDir        bool
	IsFile       bool
	IsSymlink    bool
	CreatedAtMS  int64
	ModifiedAtMS int64
}

// 数据。一次文件监听变化；Error 非空表示监听已无法继续可靠工作。
type FileWatchEvent struct {
	ChangedPaths []string
	Error        error
}

// 活对象。一条文件或目录监听；Close 幂等停止并释放底层资源。
type FileWatch interface {
	Events() <-chan FileWatchEvent
	Close() error
}

// 数据。启动一个可继续交互的本机进程。
type ProcessRequest struct {
	OwnerID string
	Dir     string
	Argv    []string
	TTY     bool
	Wait    time.Duration
}

// 数据。向已有进程写入字节，或只等待并读取新增输出。
type ProcessInteraction struct {
	OwnerID   string
	ProcessID int64
	Chars     []byte
	Wait      time.Duration
}

// 数据。一次进程操作得到的新增输出与当前状态。
type ProcessOutput struct {
	ProcessID    int64
	Output       []byte
	Exited       bool
	ExitCode     int
	OmittedBytes int64
}

// 数据。启动一条由富 Client 直接管理的 PTY 进程。
type TerminalRequest struct {
	Dir  string
	Argv []string
	Rows int
	Cols int
}

// 契约。一条由调用方拥有生命周期的 PTY 进程。
type TerminalProcess interface {
	Output() <-chan []byte
	Write(data []byte) error
	CloseInput() error
	Resize(rows int, cols int) error
	Terminate() error
	Wait(ctx context.Context) (int, error)
}

// 契约。同一份 machine 服务提供的低层 PTY 能力。
type TerminalSystem interface {
	StartTerminal(request TerminalRequest) (TerminalProcess, error)
}

// 契约。同一份 machine 服务提供的长期进程能力。
type ProcessSystem interface {
	// 进程启动与后续交互
	Exec(ctx context.Context, request ProcessRequest) (ProcessOutput, error)
	Interact(ctx context.Context, interaction ProcessInteraction) (ProcessOutput, error)
}

// 契约。文件和进程所在机器提供的操作。
type Machine interface {
	// HomeDir 返回当前机器上用户的主目录。
	HomeDir() (string, error)

	// 文件操作
	ReadFile(path string) ([]byte, error)
	ReadDir(path string) ([]DirEntry, error)
	WriteFile(path string, data []byte) error

	// 路径
	ResolvePath(workspace string, path string) string
}

// 契约。同一份 machine 服务提供的版本化文件操作、元数据与监听能力。
type FileSystem interface {
	Machine
	ReadFileVersion(path string, maxBytes int64) (FileContent, error)
	Metadata(path string) (FileMetadata, error)
	WriteFileIfUnchanged(path string, data []byte, expectedHash string) (string, error)
	RemoveFileIfUnchanged(path string, expectedHash string) error
	Watch(path string) (FileWatch, error)
}
