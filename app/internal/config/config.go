package config

import (
	"TODOLIST_Tasks/app/pkg/logging"
	"flag"
	"github.com/ilyakaznacheev/cleanenv"
	"log/slog"
	"os"
	"sync"
)

type Config struct {
	ListenConfig          ListenConfig         `yaml:"listen"`
	StoragePostgresConfig StoragePostgresTasks `yaml:"storagePostgresTasks"`
	StorageRedisConfig    StorageRedisTasks    `yaml:"storageRedisTasks"`
}

type ListenConfig struct {
	Type   string `yaml:"type" env-default:"port"`
	Port   string `yaml:"port" env-default:"8000"`
	BindIP string `yaml:"bind_ip" env-default:"127.0.0.1"`
}

type StoragePostgresTasks struct {
	Host     string `yaml:"host" env-default:"localhost"`
	Port     string `yaml:"port" env-default:"4000"`
	Database string `yaml:"database" env-default:"mydatabase1"`
	Username string `yaml:"username" env-default:"user1"`
	Password string `yaml:"password" env-default:"password1"`
}

type StorageRedisTasks struct {
	Host     string `yaml:"host" env-default:"localhost"`
	Port     string `yaml:"port" env-default:"6379"`
	Username string
	Password string `yaml:"password" env-default:"yourpassword"`
	Protocol string `yaml:"protocol" env-default:"tcp"`
}

const (
	flagConfigPathName = "config"
	envConfigPathName  = "CONFIG_PATH"
)

var instance *Config
var once sync.Once

func GetConfig() *Config {
	once.Do(func() {
		logger := logging.GetLogger()
		var configPath string
		flag.StringVar(&configPath, flagConfigPathName, "", "path to config file")
		flag.Parse()

		if path, ok := os.LookupEnv(envConfigPathName); ok {
			configPath = path
		}
		instance = &Config{}

		if readErr := cleanenv.ReadConfig(configPath, instance); readErr != nil {
			description, descErr := cleanenv.GetDescription(instance, nil)
			if descErr != nil {
				panic(descErr)
			}
			logger.Info(description)
			logger.Error("failed to read config ", slog.String("err", readErr.Error()), slog.String("path", configPath))
			os.Exit(1)
		}
	})
	return instance
}
