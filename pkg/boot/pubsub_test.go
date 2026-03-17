package boot

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 测试用自定义结构体
type testEvent struct {
	ID   int
	Name string
}

// TestPubSubInt 测试 int 类型的发布和消费
func TestPubSubInt(t *testing.T) {
	ps := NewPubSub()
	var got atomic.Int64

	sub := ps.Subscribe("int_topic", func(v int) {
		got.Store(int64(v))
	})
	sub.Start(context.Background())

	ps.Publish(context.Background(), "int_topic", 42)
	time.Sleep(50 * time.Millisecond)

	if got.Load() != 42 {
		t.Fatalf("expected 42, got %d", got.Load())
	}
	ps.Close()
}

// TestPubSubString 测试 string 类型的发布和消费
func TestPubSubString(t *testing.T) {
	ps := NewPubSub()
	var got atomic.Value

	sub := ps.Subscribe("str_topic", func(v string) {
		got.Store(v)
	})
	sub.Start(context.Background())

	ps.Publish(context.Background(), "str_topic", "hello")
	time.Sleep(50 * time.Millisecond)

	if got.Load() != "hello" {
		t.Fatalf("expected 'hello', got '%v'", got.Load())
	}
	ps.Close()
}

// TestPubSubMultiArgs 测试多参数混合类型的发布和消费
func TestPubSubMultiArgs(t *testing.T) {
	ps := NewPubSub()
	var (
		gotInt  atomic.Int64
		gotStr  atomic.Value
		gotBool atomic.Bool
	)

	sub := ps.Subscribe("multi", func(a int, b string, c bool) {
		gotInt.Store(int64(a))
		gotStr.Store(b)
		gotBool.Store(c)
	})
	sub.Start(context.Background())

	ps.Publish(context.Background(), "multi", 99, "world", true)
	time.Sleep(50 * time.Millisecond)

	if gotInt.Load() != 99 {
		t.Fatalf("expected 99, got %d", gotInt.Load())
	}
	if gotStr.Load() != "world" {
		t.Fatalf("expected 'world', got '%v'", gotStr.Load())
	}
	if !gotBool.Load() {
		t.Fatal("expected true, got false")
	}
	ps.Close()
}

// TestPubSubStruct 测试自定义结构体值类型的发布和消费
func TestPubSubStruct(t *testing.T) {
	ps := NewPubSub()
	var got atomic.Value

	sub := ps.Subscribe("struct_topic", func(e testEvent) {
		got.Store(e)
	})
	sub.Start(context.Background())

	ps.Publish(context.Background(), "struct_topic", testEvent{ID: 1, Name: "test"})
	time.Sleep(50 * time.Millisecond)

	v := got.Load().(testEvent)
	if v.ID != 1 || v.Name != "test" {
		t.Fatalf("expected {1 test}, got %+v", v)
	}
	ps.Close()
}

// TestPubSubStructPointer 测试自定义结构体指针类型的发布和消费
func TestPubSubStructPointer(t *testing.T) {
	ps := NewPubSub()
	var got atomic.Value

	sub := ps.Subscribe("ptr_topic", func(e *testEvent) {
		got.Store(e)
	})
	sub.Start(context.Background())

	ps.Publish(context.Background(), "ptr_topic", &testEvent{ID: 2, Name: "ptr"})
	time.Sleep(50 * time.Millisecond)

	v := got.Load().(*testEvent)
	if v.ID != 2 || v.Name != "ptr" {
		t.Fatalf("expected {2 ptr}, got %+v", v)
	}
	ps.Close()
}

// TestPubSubMultipleSubscribers 测试一个 topic 多个 subscriber 都能收到消息
func TestPubSubMultipleSubscribers(t *testing.T) {
	ps := NewPubSub()
	var count atomic.Int64

	for i := 0; i < 3; i++ {
		sub := ps.Subscribe("shared", func(v int) {
			count.Add(1)
		})
		sub.Start(context.Background())
	}

	ps.Publish(context.Background(), "shared", 1)
	time.Sleep(50 * time.Millisecond)

	if count.Load() != 3 {
		t.Fatalf("expected 3 subscribers to consume, got %d", count.Load())
	}
	ps.Close()
}

// TestPubSubTopicIsolation 测试不同 topic 之间消息隔离
func TestPubSubTopicIsolation(t *testing.T) {
	ps := NewPubSub()
	var got1, got2 atomic.Int64

	sub1 := ps.Subscribe("topic1", func(v int) {
		got1.Store(int64(v))
	})
	sub1.Start(context.Background())

	sub2 := ps.Subscribe("topic2", func(v int) {
		got2.Store(int64(v))
	})
	sub2.Start(context.Background())

	ps.Publish(context.Background(), "topic1", 10)
	time.Sleep(50 * time.Millisecond)

	if got1.Load() != 10 {
		t.Fatalf("topic1 subscriber expected 10, got %d", got1.Load())
	}
	if got2.Load() != 0 {
		t.Fatalf("topic2 subscriber should not receive, got %d", got2.Load())
	}
	ps.Close()
}

// TestPubSubPoolSize 测试 poolSize > 1 的并发消费
func TestPubSubPoolSize(t *testing.T) {
	ps := NewPubSub()
	var count atomic.Int64
	var wg sync.WaitGroup

	total := 50
	wg.Add(total)

	sub := ps.Subscribe("pool", func(v int) {
		defer wg.Done()
		count.Add(1)
	}, WithPoolSize(4))
	sub.Start(context.Background())

	for i := 0; i < total; i++ {
		ps.Publish(context.Background(), "pool", i)
	}

	wg.Wait()
	if count.Load() != int64(total) {
		t.Fatalf("expected %d consumed, got %d", total, count.Load())
	}
	ps.Close()
}

// TestPubSubRecovery 测试 handler panic 不会导致 worker 退出，后续消息仍能消费
func TestPubSubRecovery(t *testing.T) {
	ps := NewPubSub()
	var count atomic.Int64

	sub := ps.Subscribe("recover", func(v int) {
		if v == 1 {
			panic("boom")
		}
		count.Add(1)
	}, WithRecovery())
	sub.Start(context.Background())

	// 第一条消息触发 panic
	ps.Publish(context.Background(), "recover", 1)
	time.Sleep(50 * time.Millisecond)

	// 第二条消息应正常消费
	ps.Publish(context.Background(), "recover", 2)
	time.Sleep(50 * time.Millisecond)

	if count.Load() != 1 {
		t.Fatalf("expected 1 successful consume after panic, got %d", count.Load())
	}
	ps.Close()
}

// TestPubSubStop 测试 Stop 后 worker 正常退出
func TestPubSubStop(t *testing.T) {
	ps := NewPubSub()
	var count atomic.Int64

	sub := ps.Subscribe("stop", func(v int) {
		count.Add(1)
	})
	sub.Start(context.Background())

	ps.Publish(context.Background(), "stop", 1)
	time.Sleep(50 * time.Millisecond)
	sub.Stop()

	if count.Load() != 1 {
		t.Fatalf("expected 1, got %d", count.Load())
	}
}

// TestPubSubStartOnce 测试 Start 只启动一次，重复调用无副作用
func TestPubSubStartOnce(t *testing.T) {
	ps := NewPubSub()
	var count atomic.Int64

	sub := ps.Subscribe("once", func(v int) {
		count.Add(1)
	}, WithPoolSize(1))
	sub.Start(context.Background())
	sub.Start(context.Background()) // 重复启动

	ps.Publish(context.Background(), "once", 1)
	time.Sleep(50 * time.Millisecond)

	// 只有 1 个 worker，消费 1 次
	if count.Load() != 1 {
		t.Fatalf("expected 1, got %d", count.Load())
	}
	ps.Close()
}

// TestPubSubDefaultOverride 测试全局默认配置被局部 opts 覆盖
func TestPubSubDefaultOverride(t *testing.T) {
	ps := NewPubSub(WithPoolSize(2), WithQueueSize(10))
	var count atomic.Int64
	var wg sync.WaitGroup

	total := 20
	wg.Add(total)

	// 局部覆盖 poolSize 为 4
	sub := ps.Subscribe("override", func(v int) {
		defer wg.Done()
		count.Add(1)
	}, WithPoolSize(4))
	sub.Start(context.Background())

	for i := 0; i < total; i++ {
		ps.Publish(context.Background(), "override", i)
	}

	wg.Wait()
	if count.Load() != int64(total) {
		t.Fatalf("expected %d, got %d", total, count.Load())
	}
	ps.Close()
}

// TestPubSubPublishNoSubscriber 测试向无订阅者的 topic 发布不会报错
func TestPubSubPublishNoSubscriber(t *testing.T) {
	ps := NewPubSub()
	// 不应 panic 或报错
	ps.Publish(context.Background(), "nobody", 1, "hello")
}

// TestPubSubHandlerNotFunc 测试 Subscribe handler 不是函数时应 panic
func TestPubSubHandlerNotFunc(t *testing.T) {
	ps := NewPubSub()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for non-func handler, got nil")
		}
	}()
	ps.Subscribe("bad", "not_a_function")
}

// TestPubSubTryPublish 测试 TryPublish，遇到阻塞的 channel 时不阻塞
func TestPubSubTryPublish(t *testing.T) {
	ps := NewPubSub()
	var count atomic.Int64

	// 用于让测试等待某条消息开始执行
	startedCh := make(chan struct{})
	// 用于阻塞 worker 处理消息
	blockCh := make(chan struct{})

	// 设置 queueSize 为 0，确保无缓存时能立刻测试出效果
	sub := ps.Subscribe("try", func(v int) {
		count.Add(1)
		// 通知测试已开始处理第一个消息
		if v == 1 {
			close(startedCh)
		}
		// 阻塞住，不让处理结束
		<-blockCh
	}, WithQueueSize(0), WithPoolSize(1))

	sub.Start(context.Background())

	// 第一个消息，用独立的 goroutine 发送，以防 queueSize=0 时 Publish 阻塞测试主协程
	go func() {
		// Publish 会阻塞，直到 worker 取出消息
		_ = ps.Publish(context.Background(), "try", 1)
	}()

	// 等待 worker 拿到第一条消息并开始处理，此时 worker 阻塞在 blockCh
	<-startedCh

	// 此时尝试用 TryPublish 发第二个消息，因为 queueSize=0 且 worker 繁忙，必定触发 default
	err2 := ps.TryPublish(context.Background(), "try", 2)
	if err2 != nil {
		t.Fatalf("TryPublish returned error: %v", err2)
	}

	// 释放 blockCh，让第一个消息处理结束
	close(blockCh)

	// 给点时间确保没有其他东西被处理
	time.Sleep(50 * time.Millisecond)

	// 总消费数应当为 1，因为第二个通过 TryPublish 发现阻塞被直接遗弃了
	if count.Load() != 1 {
		t.Fatalf("expected 1 successful consume, but got %d", count.Load())
	}
	ps.Close()
}
