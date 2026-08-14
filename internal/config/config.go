package config

import "os"

type Config struct {
	HTTPAddr          string
	DatabaseURL       string
	ProviderMode      string
	FakeScenario      string
	FakeAlertScenario string
	ReadSource        string
}

func Load() Config {
	return Config{
		HTTPAddr:          env("ATLAS_HTTP_ADDR", ":8080"),
		DatabaseURL:       env("ATLAS_DATABASE_URL", "postgres://atlas:atlas_dev@127.0.0.1:15432/atlas?sslmode=disable"),
		ProviderMode:      env("ATLAS_PROVIDER_MODE", "fake"),
		FakeScenario:      env("ATLAS_FAKE_SCENARIO", "reef-healthy-baremetal"),
		FakeAlertScenario: env("ATLAS_FAKE_ALERT_SCENARIO", "osd-down-alert"),
		ReadSource:        env("ATLAS_READ_SOURCE", "provider"),
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
