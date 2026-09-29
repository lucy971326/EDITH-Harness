package oauth

import (
	"context"
	"fmt"
	"time"
)

// 数据。一次设备码轮询的结果；SlowDown 按 RFC 8628 增加或更新等待间隔。
type DeviceResult struct {
	Credential *Credential
	SlowDown   bool
	Interval   time.Duration
}

// PollDevice 在过期或取消前按服务端间隔轮询，首次请求前也等待。
func PollDevice(ctx context.Context, interval, expires time.Duration, poll func(context.Context) (DeviceResult, error)) (*Credential, error) {
	if interval < time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(expires)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("设备授权已过期，请重新登录")
		}
		wait := min(interval, remaining)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
		if time.Until(deadline) <= 0 {
			return nil, fmt.Errorf("设备授权已过期，请重新登录")
		}
		result, err := poll(ctx)
		if err != nil {
			return nil, err
		}
		if result.Credential != nil {
			return result.Credential, nil
		}
		if result.SlowDown {
			if result.Interval >= time.Second {
				interval = result.Interval
			} else {
				interval += 5 * time.Second
			}
		}
	}
}
