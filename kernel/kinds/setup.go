package kinds

// 数据。一本聊天旁的元数据，不进账本。
type Setup struct {
	Kind            string   `json:"kind"`
	Persona         string   `json:"persona"`
	Model           string   `json:"model"`
	ReasoningEffort string   `json:"reasoningEffort"`
	Tools           []string `json:"tools"`
	Workspace       string   `json:"workspace"`
}

// 契约。按 session 读写 Setup。For 必须深拷贝，至少拷 Tools。
type Setups interface {
	For(sessionID string) (Setup, error)
	Put(sessionID string, s Setup) error
}
