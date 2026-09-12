package main

import (
	"errors"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	HTTP struct {
		Port                           string
		ReadTimeoutInSeconds           int64
		WriteTimeoutInSeconds          int64
		IdleTimeoutInSeconds           int64
		DefaultShutdownPeriodInSeconds int64
	}
	SMTP struct {
		Host      string
		Port      int
		Username  string
		Password  string
		Sender    string
		TLSPolicy string
	}
	Github struct {
		URL           string
		WebhookSecret string
		PrivateKey    string
	}
	Blog struct {
		PostDir     string
		Title       string
		Author      string
		Description string
		URL         string
		Secret      string
	}
	Meilisearch struct {
		Host string
		Key  string
	}
}

func applyDefaults(v *viper.Viper) {
	v.SetDefault("http.readTimeoutInSeconds", 10)
	v.SetDefault("http.writeTimeoutInSeconds", 10)
	v.SetDefault("http.idleTimeoutInSeconds", 60)
	v.SetDefault("http.defaultShutdownPeriodInSeconds", 30)
	v.SetDefault("smtp.tlsPolicy", "mandatory")
}

func LoadConfig() (Config, error) {
	var cfg Config

	v := viper.New()
	applyDefaults(v)
	v.SetConfigName("app")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	err := v.ReadInConfig()
	if err != nil {
		if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
			return cfg, err
		}
	}

	v.SetEnvPrefix("golb")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	for _, key := range []string{
		"http.port",
		"http.readTimeoutInSeconds",
		"http.writeTimeoutInSeconds",
		"http.idleTimeoutInSeconds",
		"http.defaultShutdownPeriodInSeconds",
		"smtp.host",
		"smtp.port",
		"smtp.username",
		"smtp.password",
		"smtp.sender",
		"smtp.tlsPolicy",
		"github.url",
		"github.webhookSecret",
		"github.privateKey",
		"blog.postDir",
		"blog.title",
		"blog.author",
		"blog.description",
		"blog.url",
		"blog.secret",
		"meilisearch.host",
		"meilisearch.key",
	} {
		if err := v.BindEnv(key); err != nil {
			return cfg, err
		}
	}

	err = v.Unmarshal(&cfg)
	if err != nil {
		return cfg, err
	}

	return cfg, nil
}
