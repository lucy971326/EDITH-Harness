package persist

import (
	"encoding/json"
	"time"
)

// 数据。账本里的一个节点。Body 是不拆的 JSON。
type Node struct {
	ID     string          `json:"id"`
	Parent string          `json:"parent"`
	Seq    uint64          `json:"seq"`
	Body   json.RawMessage `json:"body"`
}

// 数据。一本账在磁盘上的样子。Nodes 是文件顺序。
type Tree struct {
	ID    string
	Nodes []Node
}

// 数据。一本会话的持久化元数据。元数据文件是会话存在的依据。
type Meta struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
}

// 契约。只负责账本字节。不认识 Append / History / Branch。
// Add 写穿一个节点（jsonl 一行）。Save 留给整棵重写。
type Persistence interface {
	// 会话元数据（会话存在的依据与列表）
	List() ([]Meta, error)
	LoadMeta(id string) (Meta, error)
	SaveMeta(meta Meta) error
	DeleteMeta(id string) error

	// 账本树与节点数据（按行追加与整树加载）
	Load(id string) (*Tree, error)
	Save(id string, tree *Tree) error
	Add(id string, node Node) error
}
