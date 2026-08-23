package llm

import "fmt"

// 错误性质：调用方靠它决定下一步（限流可以等，鉴权错了别再试）。
const (
	ErrNetwork    = "network"     // 网络不通、超时
	ErrRateLimit  = "rate_limit"  // 被限流，可以等一等再试
	ErrAuth       = "auth"        // 密钥不对、没权限——别重试，去修配置
	ErrBadRequest = "bad_request" // 请求本身不合法，重试也没用
	ErrServer     = "server"      // 服务商内部错误
	ErrCancelled  = "cancelled"   // 调用方主动取消
	ErrUnknown    = "unknown"     // 说不清是什么
)

// Error 是 llm 出口的统一错误：哪家出的、什么性质、原话是什么。
// 适配器内部抛什么都行，出了插座排都是这一个样子，消费方不用认识各家错误。
type Error struct {
	Provider string // 哪家
	Kind     string // 什么性质，取上面的常量
	Message  string // 原话（服务商报错或底层错误信息）
}

// Error 返回给人看的整句。
func (e *Error) Error() string {
	return fmt.Sprintf("模型 %s 出错（%s）：%s", e.Provider, e.Kind, e.Message)
}

// NewError 适配器作者用它把自家错误翻译成统一错误。
func NewError(provider string, kind string, message string) *Error {
	return &Error{Provider: provider, Kind: kind, Message: message}
}

// wrapError 出口包装：适配器返回的已是统一错误就原样放行，其余一律包成 unknown。
func wrapError(provider string, err error) error {
	if err == nil {
		return nil
	}
	if typed, ok := err.(*Error); ok {
		return typed
	}
	return &Error{Provider: provider, Kind: ErrUnknown, Message: err.Error()}
}
