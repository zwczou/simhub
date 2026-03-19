package simd

import (
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/spf13/viper"
)

var supportedConfigDirs = []string{"configs", "contrib", ".", "/etc/simd"}

type option struct{}

func NewOption() *option {
	return &option{}
}

func (opts *option) Load(path string) error {
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()
	viper.SetEnvPrefix("SIM")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	for _, path := range supportedConfigDirs {
		viper.AddConfigPath(path)
	}
	viper.SetConfigName("common")
	if err := viper.MergeInConfig(); err != nil {
		return err
	}

	if path != "" {
		viper.SetConfigFile(path)
	} else {
		viper.SetConfigName("simd")
	}
	if err := viper.MergeInConfig(); err != nil {
		return err
	}

	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		log.Warn().Str("name", e.Name).Msg("config file changed")
		err := opts.initLogger()
		if err != nil {
			log.Error().Err(err).Msg("init logger failed")
		}
	})
	return opts.initLogger()
}

func (opts *option) initLogger() error {
	level, err := zerolog.ParseLevel(viper.GetString("log.level"))
	if err != nil {
		return err
	}
	zerolog.SetGlobalLevel(level)
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		count := 0
		for i := len(file) - 1; i > 0; i-- {
			if file[i] == '/' {
				if count > 0 {
					file = file[i+1:]
					break
				}
				count++
			}
		}
		return file + ":" + strconv.Itoa(line)
	}

	var out io.Writer
	out = os.Stderr

	// 是否启动控制台输出
	if viper.GetString("log.format") != "json" {
		out = zerolog.ConsoleWriter{Out: out}
	}

	logger := zerolog.New(out).With().Caller().Timestamp().Str("hostname", lo.Must(os.Hostname())).Logger()
	logger.Info().Str("new_level", level.String()).Msg("set log level")

	log.Logger = logger
	zerolog.DefaultContextLogger = &logger
	return nil
}
