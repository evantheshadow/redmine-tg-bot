package main

import (
	"log"

	toml "github.com/BurntSushi/toml"
)

type ProxyConfig struct {
	Scheme   string
	Host     string
	Port     int
	User     string
	Password string
}

type Config struct {
	DbFile               string
	WebhookHost          string
	WebhookPort          int
	TgToken              string
	RedmineHost          string
	RedmineAPIHost       string
	RedmineToken         string
	Debug                string
	NotificationTemplate string
	QueueSize            int
	Proxy                ProxyConfig `toml:"Proxy"`
}

func parseConfig(configFile string) Config {
	var config Config
	if _, err := toml.DecodeFile(configFile, &config); err != nil {
		log.Panic(err)
	}
	return config
}
