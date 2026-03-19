package redlb

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	json "github.com/goccy/go-json"
	"github.com/redis/go-redis/v9"
)

// TestSplitHostPortPort 验证监听地址解析逻辑。
func TestSplitHostPortPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		addr    string
		want    int
		wantErr bool
	}{
		{name: "host and port", addr: "127.0.0.1:9988", want: 9988},
		{name: "only port with colon", addr: ":9988", want: 9988},
		{name: "ipv6", addr: "[::1]:9988", want: 9988},
		{name: "plain port should fail", addr: "9988", wantErr: true},
		{name: "invalid", addr: "bad", wantErr: true},
		{name: "out of range", addr: "70000", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := splitHostPortPort(tt.addr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.addr)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitHostPortPort(%q) error: %v", tt.addr, err)
			}
			if got != tt.want {
				t.Fatalf("splitHostPortPort(%q)=%d, want %d", tt.addr, got, tt.want)
			}
		})
	}
}

// TestRegisterDiscoverAndStop 验证注册、发现、续期与停止会同步维护索引。
func TestRegisterDiscoverAndStop(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	reg := NewRegistry(
		rdb,
		WithPrefix("test:redlb"),
		WithTtl(3*time.Second),
		WithHeartbeatInterval(300*time.Millisecond),
		WithOperationTimeout(500*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lease, ep, err := reg.Register(ctx, Registration{
		ServiceName: "user.service",
		Ip:          "10.10.0.8",
		Hostname:    "node-a",
		GrpcAddr:    ":9988",
		HttpAddr:    ":9989",
	})
	if err != nil {
		t.Fatalf("Register() error: %v", err)
	}

	if ep.Ip != "10.10.0.8" {
		t.Fatalf("endpoint ip=%q, want 10.10.0.8", ep.Ip)
	}
	if ep.Hostname != "node-a" {
		t.Fatalf("endpoint hostname=%q, want node-a", ep.Hostname)
	}
	if ep.GrpcPort != 9988 || ep.HttpPort != 9989 {
		t.Fatalf("endpoint ports grpc=%d http=%d", ep.GrpcPort, ep.HttpPort)
	}

	eps, err := reg.Discover(context.Background(), "user.service")
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if len(eps) != 1 {
		t.Fatalf("Discover() len=%d, want 1", len(eps))
	}

	endpointKey := reg.serviceKey("user.service", ep.InstanceId)
	indexKey := reg.indexKey("user.service")

	keys := mr.Keys()
	if len(keys) != 2 {
		t.Fatalf("redis keys=%d, want 2", len(keys))
	}

	raw, err := rdb.Get(context.Background(), endpointKey).Result()
	if err != nil {
		t.Fatalf("redis get error: %v", err)
	}
	var stored Endpoint
	if err = json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatalf("unmarshal redis endpoint error: %v", err)
	}
	if stored.Hostname != "node-a" {
		t.Fatalf("stored hostname=%q, want node-a", stored.Hostname)
	}

	members, err := rdb.SMembers(context.Background(), indexKey).Result()
	if err != nil {
		t.Fatalf("read index members error: %v", err)
	}
	if len(members) != 1 || members[0] != endpointKey {
		t.Fatalf("index members=%v, want [%s]", members, endpointKey)
	}

	time.Sleep(1200 * time.Millisecond)
	if !mr.Exists(endpointKey) {
		t.Fatalf("key %q expired unexpectedly, heartbeat not working", endpointKey)
	}
	if !mr.Exists(indexKey) {
		t.Fatalf("index key %q expired unexpectedly, heartbeat not working", indexKey)
	}

	if err = lease.Stop(context.Background()); err != nil {
		t.Fatalf("lease stop error: %v", err)
	}
	if mr.Exists(endpointKey) {
		t.Fatalf("key %q should be deleted after Stop", endpointKey)
	}

	members, err = rdb.SMembers(context.Background(), indexKey).Result()
	if err != nil && err != redis.Nil {
		t.Fatalf("read index members after stop error: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("index members after stop=%v, want empty", members)
	}
}

// TestDiscoverCleansStaleMembers 验证发现流程会清理失效索引成员。
func TestDiscoverCleansStaleMembers(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	reg := NewRegistry(
		rdb,
		WithPrefix("test:redlb"),
		WithTtl(3*time.Second),
		WithHeartbeatInterval(300*time.Millisecond),
	)

	lease, ep, err := reg.Register(context.Background(), Registration{
		ServiceName: "clean.service",
		InstanceId:  "node-1",
		Ip:          "10.10.0.9",
		Hostname:    "node-clean",
		GrpcAddr:    ":9988",
		HttpAddr:    ":9989",
	})
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	defer func() { _ = lease.Stop(context.Background()) }()

	indexKey := reg.indexKey("clean.service")
	staleKey := "test:redlb:clean.service:stale"
	if err := rdb.SAdd(context.Background(), indexKey, staleKey).Err(); err != nil {
		t.Fatalf("seed stale member error: %v", err)
	}

	eps, err := reg.Discover(context.Background(), "clean.service")
	if err != nil {
		t.Fatalf("discover error: %v", err)
	}
	if len(eps) != 1 || eps[0].InstanceId != ep.InstanceId {
		t.Fatalf("discover endpoints=%+v, want only %s", eps, ep.InstanceId)
	}

	members, err := rdb.SMembers(context.Background(), indexKey).Result()
	if err != nil {
		t.Fatalf("read index members error: %v", err)
	}
	if len(members) != 1 || members[0] != reg.serviceKey("clean.service", ep.InstanceId) {
		t.Fatalf("index members after cleanup=%v", members)
	}
}

// TestRegisterHostnameFallback 验证主机名会在缺失时回退到本机。
func TestRegisterHostnameFallback(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	reg := NewRegistry(
		rdb,
		WithPrefix("test:redlb"),
		WithTtl(3*time.Second),
		WithHeartbeatInterval(300*time.Millisecond),
	)

	lease, ep, err := reg.Register(context.Background(), Registration{
		ServiceName: "fallback.service",
		Ip:          "10.10.0.9",
		GrpcAddr:    ":9988",
		HttpAddr:    ":9989",
	})
	if err != nil {
		t.Fatalf("Register with hostname fallback error: %v", err)
	}
	defer func() { _ = lease.Stop(context.Background()) }()

	wantHostname, _ := os.Hostname()
	if ep.Hostname != wantHostname {
		t.Fatalf("endpoint hostname=%q, want %q", ep.Hostname, wantHostname)
	}
}

// TestRegisterRequiresServiceName 验证服务名为空时会报错。
func TestRegisterRequiresServiceName(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(nil)
	_, _, err := reg.Register(context.Background(), Registration{
		Ip:       "10.10.0.9",
		GrpcAddr: ":9988",
	})
	if err != ErrServiceNameRequired {
		t.Fatalf("expected ErrServiceNameRequired, got %v", err)
	}
}

// TestRegisterRequiresIp 验证注册缺少 IP 时会报错。
func TestRegisterRequiresIp(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(nil)
	_, _, err := reg.Register(context.Background(), Registration{
		ServiceName: "svc.test",
		GrpcAddr:    ":9988",
	})
	if err != ErrIpRequired {
		t.Fatalf("expected ErrIpRequired, got %v", err)
	}
}

// TestRegisterRequiresGrpcAddr 验证注册缺少 gRPC 地址时会报错。
func TestRegisterRequiresGrpcAddr(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(nil)
	_, _, err := reg.Register(context.Background(), Registration{
		ServiceName: "svc.test",
		Ip:          "10.10.0.9",
	})
	if err != ErrGrpcAddrRequired {
		t.Fatalf("expected ErrGrpcAddrRequired, got %v", err)
	}
}
