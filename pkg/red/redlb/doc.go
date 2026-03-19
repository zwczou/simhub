// Package redlb 提供基于 Redis 的服务注册与服务发现，包含 gRPC resolver。
//
// 基本使用：
//
//	reg := redlb.NewRegistry(rdb)
//	lease, _, err := reg.Register(ctx, redlb.Registration{
//		ServiceName: "user.service",
//		Ip:          "10.10.0.8",
//		GrpcAddr:    ":9988",
//		HttpAddr:    ":9989",
//	})
//	if err != nil {
//		panic(err)
//	}
//	defer lease.Stop(context.Background())
//
//	redlb.RegisterGrpcResolver(reg)
//	conn, err := grpc.Dial(
//		"redlb:///user.service",
//		grpc.WithTransportCredentials(insecure.NewCredentials()),
//		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
//	)
//
// 默认行为：
// - 注册信息由调用方显式提供。
// - Hostname 为空时尝试使用 os.Hostname()。
// - 4 秒心跳续期，12 秒 Ttl。
package redlb
