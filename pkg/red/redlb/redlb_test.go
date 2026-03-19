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

	keys := mr.Keys()
	if len(keys) != 1 {
		t.Fatalf("redis keys=%d, want 1", len(keys))
	}

	raw, err := rdb.Get(context.Background(), keys[0]).Result()
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

	time.Sleep(1200 * time.Millisecond)
	if !mr.Exists(keys[0]) {
		t.Fatalf("key %q expired unexpectedly, heartbeat not working", keys[0])
	}

	if err = lease.Stop(context.Background()); err != nil {
		t.Fatalf("lease stop error: %v", err)
	}
	if mr.Exists(keys[0]) {
		t.Fatalf("key %q should be deleted after Stop", keys[0])
	}
}

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
