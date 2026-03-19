package redlb

import (
	"context"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/serviceconfig"
)

func TestParseServiceName(t *testing.T) {
	t.Parallel()

	u1, _ := url.Parse("redlb:///service.name")
	if got := parseServiceName(resolver.Target{URL: *u1}); got != "service.name" {
		t.Fatalf("parseServiceName(redlb:///service.name)=%q", got)
	}

	u2, _ := url.Parse("redlb://service.name")
	if got := parseServiceName(resolver.Target{URL: *u2}); got != "service.name" {
		t.Fatalf("parseServiceName(redlb://service.name)=%q", got)
	}
}

func TestGrpcResolverBuildAndResolveNow(t *testing.T) {
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

	ctx := context.Background()
	lease1, _, err := reg.Register(ctx, Registration{
		ServiceName: "svc.test",
		InstanceId:  "n1",
		Ip:          "10.0.0.1",
		Hostname:    "node-1",
		GrpcAddr:    ":9988",
		HttpAddr:    ":9989",
	})
	if err != nil {
		t.Fatalf("Register #1 error: %v", err)
	}
	defer func() { _ = lease1.Stop(context.Background()) }()

	cc := &fakeClientConn{}
	builder := NewGrpcResolverBuilder(reg)
	u, _ := url.Parse("redlb:///svc.test")
	res, err := builder.Build(resolver.Target{URL: *u}, cc, resolver.BuildOptions{})
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	defer res.Close()

	waitAddresses(t, cc, 1)

	lease2, _, err := reg.Register(ctx, Registration{
		ServiceName: "svc.test",
		InstanceId:  "n2",
		Ip:          "10.0.0.2",
		Hostname:    "node-2",
		GrpcAddr:    ":9990",
		HttpAddr:    ":9991",
	})
	if err != nil {
		t.Fatalf("Register #2 error: %v", err)
	}
	defer func() { _ = lease2.Stop(context.Background()) }()

	res.ResolveNow(resolver.ResolveNowOptions{})
	waitAddresses(t, cc, 2)
}

func TestGrpcResolverRejectsEmptyServiceName(t *testing.T) {
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

	lease, _, err := reg.Register(context.Background(), Registration{
		ServiceName: "boot.svc",
		Ip:          "10.0.0.3",
		Hostname:    "node-3",
		GrpcAddr:    ":9988",
		HttpAddr:    ":9989",
	})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	defer func() { _ = lease.Stop(context.Background()) }()

	cc := &fakeClientConn{}
	builder := NewGrpcResolverBuilder(reg)
	u, _ := url.Parse("redlb:///")
	res, err := builder.Build(resolver.Target{URL: *u}, cc, resolver.BuildOptions{})
	if err == nil {
		res.Close()
		t.Fatal("expected Build() error for empty service name")
	}
}

func waitAddresses(t *testing.T, cc *fakeClientConn, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(cc.addresses()) == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("wait addresses timeout, got %d, want %d", len(cc.addresses()), want)
}

type fakeClientConn struct {
	mu    sync.Mutex
	state resolver.State
	err   error
}

func (f *fakeClientConn) UpdateState(state resolver.State) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = state
	return nil
}

func (f *fakeClientConn) ReportError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeClientConn) NewAddress(addresses []resolver.Address) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state.Addresses = addresses
}

func (f *fakeClientConn) NewServiceConfig(_ string) {}

func (f *fakeClientConn) ParseServiceConfig(_ string) *serviceconfig.ParseResult {
	return &serviceconfig.ParseResult{}
}

func (f *fakeClientConn) addresses() []resolver.Address {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]resolver.Address, len(f.state.Addresses))
	copy(out, f.state.Addresses)
	return out
}
