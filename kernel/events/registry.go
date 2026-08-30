// Package events 提供进程内的同步事件登记处。
package events

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

// 数据。登记处内部的一条同步监听。
type listener struct {
	id   uint64
	call func(context.Context, any) error
}

// 活对象。挂在 Host 上、按 Go 类型区分事件的监听登记处。
type Registry struct {
	mu        sync.RWMutex
	nextID    uint64
	listeners map[reflect.Type][]listener
}

// NewRegistry 造一张空的事件监听登记处。
func NewRegistry() *Registry {
	return &Registry{listeners: make(map[reflect.Type][]listener)}
}

// Subscribe 按事件类型登记同步监听，并返回幂等注销函数。
func Subscribe[T any](registry *Registry, handler func(context.Context, T) error) (func(), error) {
	if registry == nil {
		return nil, fmt.Errorf("events: nil registry")
	}
	if handler == nil {
		return nil, fmt.Errorf("events: nil handler")
	}

	typ := reflect.TypeFor[T]()
	registry.mu.Lock()
	registry.nextID++
	id := registry.nextID
	registry.listeners[typ] = append(registry.listeners[typ], listener{
		id: id,
		call: func(ctx context.Context, event any) error {
			return handler(ctx, event.(T))
		},
	})
	registry.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			registry.remove(typ, id)
		})
	}, nil
}

// Publish 按登记顺序同步通知同一类型的全部监听者，并汇总错误。
func Publish[T any](ctx context.Context, registry *Registry, event T) error {
	if registry == nil {
		return fmt.Errorf("events: nil registry")
	}

	typ := reflect.TypeFor[T]()
	registry.mu.RLock()
	registered := registry.listeners[typ]
	listeners := append([]listener(nil), registered...)
	registry.mu.RUnlock()

	var errs []error
	for _, item := range listeners {
		err := item.call(ctx, event)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *Registry) remove(typ reflect.Type, id uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := r.listeners[typ]
	for i, item := range items {
		if item.id != id {
			continue
		}
		items = append(items[:i], items[i+1:]...)
		if len(items) == 0 {
			delete(r.listeners, typ)
		} else {
			r.listeners[typ] = items
		}
		return
	}
}
