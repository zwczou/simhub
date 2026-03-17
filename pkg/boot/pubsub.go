package boot

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
)

// subOptions 是订阅配置项
type subOptions struct {
	poolSize  int
	queueSize int
	recovery  bool
}

var defaultSubOptions = subOptions{
	poolSize:  1,
	queueSize: 128,
	recovery:  false,
}

// SubOption 是订阅配置函数
type SubOption func(*subOptions)

// WithPoolSize 设置 worker 并发数量
func WithPoolSize(n int) SubOption {
	return func(o *subOptions) { o.poolSize = n }
}

// WithQueueSize 设置 channel 缓冲大小
func WithQueueSize(n int) SubOption {
	return func(o *subOptions) { o.queueSize = n }
}

// WithRecovery 开启 panic recovery，防止消费 panic 导致 worker 退出
func WithRecovery() SubOption {
	return func(o *subOptions) { o.recovery = true }
}

// message 是发布到 subscriber 的内部消息
type message struct {
	ctx  context.Context
	args []reflect.Value
}

// topicEntry 是每个 topic 的订阅列表，拥有独立的读写锁避免全局竞争
type topicEntry struct {
	mu           sync.RWMutex
	subs         []*Subscriber
	argTypes     []reflect.Type
	signatureSet bool
}

// subscribers 返回当前订阅者列表的副本
func (te *topicEntry) subscribers() []*Subscriber {
	te.mu.RLock()
	subs := make([]*Subscriber, len(te.subs))
	copy(subs, te.subs)
	te.mu.RUnlock()
	return subs
}

// matchSignature 校验 topic 的 handler 签名是否一致。
func (te *topicEntry) matchSignature(fnType reflect.Type) bool {
	if !te.signatureSet {
		return true
	}
	if len(te.argTypes) != fnType.NumIn() {
		return false
	}
	for i := range te.argTypes {
		if te.argTypes[i] != fnType.In(i) {
			return false
		}
	}
	return true
}

// setSignature 记录 topic 的 handler 参数签名。
func (te *topicEntry) setSignature(fnType reflect.Type) {
	te.argTypes = make([]reflect.Type, fnType.NumIn())
	for i := range te.argTypes {
		te.argTypes[i] = fnType.In(i)
	}
	te.signatureSet = true
}

// removeSubscriber 将订阅者从 topic 中移除。
func (te *topicEntry) removeSubscriber(target *Subscriber) {
	for i, sub := range te.subs {
		if sub == target {
			te.subs = append(te.subs[:i], te.subs[i+1:]...)
			return
		}
	}
}

// PubSub 是基于 topic 的发布订阅管理器
// 每个 topic 拥有独立的读写锁，不同 topic 之间零竞争
type PubSub struct {
	mu       sync.RWMutex
	topics   map[string]*topicEntry
	defaults subOptions
	exitCh   chan struct{}
	closed   atomic.Bool
}

// NewPubSub 创建发布订阅管理器，可传入全局默认配置
func NewPubSub(opts ...SubOption) *PubSub {
	defaults := defaultSubOptions
	for _, o := range opts {
		o(&defaults)
	}
	return &PubSub{
		topics:   make(map[string]*topicEntry),
		defaults: defaults,
		exitCh:   make(chan struct{}),
	}
}

// getOrCreateTopic 获取或创建 topic 条目
func (ps *PubSub) getOrCreateTopic(topic string) *topicEntry {
	ps.mu.RLock()
	entry, ok := ps.topics[topic]
	ps.mu.RUnlock()
	if ok {
		return entry
	}

	ps.mu.Lock()
	entry, ok = ps.topics[topic]
	if !ok {
		entry = &topicEntry{}
		ps.topics[topic] = entry
	}
	ps.mu.Unlock()
	return entry
}

// Publish 向指定 topic 发布消息，所有参数通过反射传递给订阅者的 handler
// 当 channel 满时会阻塞，直到有空间或 ctx 被取消或 PubSub 已关闭
func (ps *PubSub) Publish(ctx context.Context, topic string, args ...any) error {
	if ps.closed.Load() {
		return ErrPubSubClosed
	}

	ps.mu.RLock()
	entry, ok := ps.topics[topic]
	ps.mu.RUnlock()
	if !ok {
		return nil
	}

	vals, err := ps.buildArgs(args, entry)
	if err != nil {
		return err
	}
	msg := &message{ctx: ctx, args: vals}

	entry.mu.RLock()
	defer entry.mu.RUnlock()

	for _, sub := range entry.subs {
		select {
		case sub.ch <- msg:
		case <-ps.exitCh:
			return ErrPubSubClosed
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// TryPublish 向指定 topic 发布消息，所有参数通过反射传递给订阅者的 handler
// 当某个订阅者的 channel 满时会直接跳过该订阅者，不会阻塞
func (ps *PubSub) TryPublish(ctx context.Context, topic string, args ...any) error {
	if ps.closed.Load() {
		return ErrPubSubClosed
	}

	ps.mu.RLock()
	entry, ok := ps.topics[topic]
	ps.mu.RUnlock()
	if !ok {
		return nil
	}

	vals, err := ps.buildArgs(args, entry)
	if err != nil {
		return err
	}
	msg := &message{ctx: ctx, args: vals}

	entry.mu.RLock()
	defer entry.mu.RUnlock()

	for _, sub := range entry.subs {
		select {
		case sub.ch <- msg:
		case <-ps.exitCh:
			return ErrPubSubClosed
		case <-ctx.Done():
			return ctx.Err()
		default:
			// 阻塞时立即尝试下一个，不等待
		}
	}
	return nil
}

// Subscribe 创建并注册一个订阅者，handler 必须是函数类型
// 配置项会合并全局默认值和局部 opts，局部优先
func (ps *PubSub) Subscribe(topic string, handler any, opts ...SubOption) *Subscriber {
	rv := reflect.ValueOf(handler)
	if rv.Kind() != reflect.Func {
		panic("boot: handler must be a function")
	}
	fnType := rv.Type()

	o := ps.defaults
	for _, opt := range opts {
		opt(&o)
	}

	sub := &Subscriber{
		topic:   topic,
		entry:   nil,
		handler: rv,
		numIn:   fnType.NumIn(),
		ch:      make(chan *message, o.queueSize),
		opts:    o,
	}

	entry := ps.getOrCreateTopic(topic)
	entry.mu.Lock()
	if !entry.matchSignature(fnType) {
		entry.mu.Unlock()
		panic(fmt.Errorf("boot: subscribe topic %q: %w", topic, ErrPubSubSignatureMismatch))
	}
	if !entry.signatureSet {
		entry.setSignature(fnType)
	}
	entry.subs = append(entry.subs, sub)
	sub.entry = entry
	entry.mu.Unlock()

	log.Info().Str("topic", topic).Msg("subscriber registered")
	return sub
}

// Close 关闭 PubSub，停止接收新消息并等待所有 worker 退出
func (ps *PubSub) Close() {
	if ps.closed.CompareAndSwap(false, true) {
		close(ps.exitCh)

		ps.mu.RLock()
		entries := make([]*topicEntry, 0, len(ps.topics))
		for _, entry := range ps.topics {
			entries = append(entries, entry)
		}
		ps.mu.RUnlock()

		for _, entry := range entries {
			for _, sub := range entry.subscribers() {
				sub.Stop()
			}
		}
	}
}

// Subscriber 是一个 topic 的消费者，包含 worker pool 并发消费消息
type Subscriber struct {
	topic   string
	entry   *topicEntry
	handler reflect.Value
	numIn   int
	ch      chan *message
	opts    subOptions
	started atomic.Bool
	wg      sync.WaitGroup
	cancel  context.CancelFunc
}

// Start 启动 worker pool 开始消费消息，使用 atomic.Bool 确保只启动一次
func (s *Subscriber) Start(ctx context.Context) {
	if s.started.CompareAndSwap(false, true) {
		ctx, s.cancel = context.WithCancel(ctx)
		s.wg.Add(s.opts.poolSize)
		for range s.opts.poolSize {
			go s.worker(ctx)
		}
		log.Ctx(ctx).Info().Str("topic", s.topic).Int("pool_size", s.opts.poolSize).Msg("subscriber started")
	}
}

// worker 是消费协程，持续从 channel 读取消息并调用 handler
func (s *Subscriber) worker(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case msg, ok := <-s.ch:
			if !ok {
				return
			}
			s.handle(msg)
		case <-ctx.Done():
			return
		}
	}
}

// handle 执行 handler 调用，根据配置决定是否 recover panic
func (s *Subscriber) handle(msg *message) {
	if s.opts.recovery {
		defer func() {
			if r := recover(); r != nil {
				log.Ctx(msg.ctx).Error().Interface("recover", r).Str("topic", s.topic).Msg("subscriber panic recovered")
			}
		}()
	}

	if len(msg.args) != s.numIn {
		log.Ctx(msg.ctx).Error().
			Str("topic", s.topic).
			Int("expected", s.numIn).
			Int("got", len(msg.args)).
			Msg("argument count mismatch")
		return
	}

	s.handler.Call(msg.args)
}

// Stop 停止消费并等待所有 worker 退出
func (s *Subscriber) Stop() {
	if s.entry != nil {
		s.entry.mu.Lock()
		s.entry.removeSubscriber(s)
		s.entry.mu.Unlock()
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

// buildArgs 根据 topic 签名构造可投递的参数值。
func (ps *PubSub) buildArgs(args []any, entry *topicEntry) ([]reflect.Value, error) {
	entry.mu.RLock()
	argTypes := make([]reflect.Type, len(entry.argTypes))
	copy(argTypes, entry.argTypes)
	entry.mu.RUnlock()

	if len(args) != len(argTypes) {
		return nil, fmt.Errorf("boot: publish expected %d args, got %d: %w", len(argTypes), len(args), ErrPubSubArgumentMismatch)
	}

	vals := make([]reflect.Value, len(args))
	for i, a := range args {
		if a == nil {
			return nil, fmt.Errorf("boot: publish arg %d is nil: %w", i, ErrPubSubArgumentMismatch)
		}

		v := reflect.ValueOf(a)
		if !v.Type().AssignableTo(argTypes[i]) {
			return nil, fmt.Errorf("boot: publish arg %d type %s not assignable to %s: %w", i, v.Type(), argTypes[i], ErrPubSubArgumentMismatch)
		}
		vals[i] = v
	}

	return vals, nil
}
