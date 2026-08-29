package persist

import "encoding/json"

// 数据。账本里的一个节点。Body 是不拆的 JSON。
type Node struct {
	ID     string          `json:"id"`
	Parent string          `json:"parent"`
	Body   json.RawMessage `json:"body"`
}

// 数据。一本账在磁盘上的样子。Nodes 是文件顺序。
type Tree struct {
	ID    string
	Nodes []Node
}

// 数据。列表里的一行。
type Meta struct {
	ID string
}

// 契约。只负责账本字节。不认识 Append / History / Branch。
// Add 写穿一个节点（jsonl 一行）。Save 留给整棵重写。
type Persistence interface {
	Load(id string) (*Tree, error)
	Save(id string, tree *Tree) error
	List() ([]Meta, error)
	Add(id string, node Node) error
}
