package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"

	"github.com/iot/simhub/pkg/boot"
	"github.com/iot/simhub/simd"
)

var (
	cfgName     string
	versionInfo bool
)

// main 是 simd 服务的命令行入口。
func main() {
	flag.BoolVar(&versionInfo, "v", false, "显示版本信息")
	flag.StringVar(&cfgName, "cfg", "", "配置文件路径")
	flag.Parse()

	// 显示版本信息
	if versionInfo {
		fmt.Println(boot.Read().String())
		return
	}

	opts := simd.NewOption()
	err := opts.Load(cfgName)
	if err != nil {
		log.Fatal().Err(err).Msg("load config failed")
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	srv := simd.New(opts)
	srv.Main()
	<-signalChan
	srv.Exit()
}
