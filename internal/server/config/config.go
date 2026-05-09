package config

import (
	"log"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Agent    AgentConfig
}

type ServerConfig struct {
	Host    string
	Port    int
	WSPort  int    `mapstructure:"ws_port"`
	TLSCert string `mapstructure:"tls_cert"`
	TLSKey  string `mapstructure:"tls_key"`
}

type DatabaseConfig struct {
	Driver string
	DSN    string
}

type AgentConfig struct {
	SecretKey         string `mapstructure:"secret_key"`
	VirtualNetwork    string `mapstructure:"virtual_network"`
	HeartbeatInterval int    `mapstructure:"heartbeat_interval"`
	BinaryDownloadURL string `mapstructure:"binary_download_url"`
}

func Load() *Config {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("/etc/mesnet/")

	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.ws_port", 443)
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "mesnet.db")
	v.SetDefault("agent.virtual_network", "10.100.0.0/16")
	v.SetDefault("agent.heartbeat_interval", 30)

	if err := v.ReadInConfig(); err != nil {
		log.Printf("config file not found, using defaults: %v", err)
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		log.Fatalf("unmarshal config failed: %v", err)
	}

	return cfg
}
