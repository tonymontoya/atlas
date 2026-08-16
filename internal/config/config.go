package config

import (
	"os"
	"strconv"
)

type Config struct {
	HTTPAddr          string
	DatabaseURL       string
	ProviderMode      string
	FakeScenario      string
	FakeAlertScenario string
	FakeAgentScenario string
	ReadSource        string
	AgentMode         string
	OIDCIssuer        string
	OIDCAudience      string
	OIDCJWKSURL       string

	CephDashboardURL         string
	CephDashboardUser        string
	CephDashboardPassword    string
	CephClusterName          string
	CephDashboardInsecureTLS bool
}

func Load() Config {
	return Config{
		HTTPAddr:          env("ATLAS_HTTP_ADDR", ":8080"),
		DatabaseURL:       env("ATLAS_DATABASE_URL", "postgres://atlas:atlas_dev@127.0.0.1:15432/atlas?sslmode=disable"),
		ProviderMode:      env("ATLAS_PROVIDER_MODE", "fake"),
		FakeScenario:      env("ATLAS_FAKE_SCENARIO", "reef-healthy-baremetal"),
		FakeAlertScenario: env("ATLAS_FAKE_ALERT_SCENARIO", "osd-down-alert"),
		FakeAgentScenario: env("ATLAS_FAKE_AGENT_SCENARIO", ""),
		ReadSource:        env("ATLAS_READ_SOURCE", "provider"),
		AgentMode:         env("ATLAS_AGENT_MODE", "disabled"),
		OIDCIssuer:        env("ATLAS_OIDC_ISSUER", ""),
		OIDCAudience:      env("ATLAS_OIDC_AUDIENCE", ""),
		OIDCJWKSURL:       env("ATLAS_OIDC_JWKS_URL", ""),

		CephDashboardURL:         env("ATLAS_CEPH_DASHBOARD_URL", ""),
		CephDashboardUser:        env("ATLAS_CEPH_DASHBOARD_USER", ""),
		CephDashboardPassword:    env("ATLAS_CEPH_DASHBOARD_PASSWORD", ""),
		CephClusterName:          env("ATLAS_CEPH_CLUSTER_NAME", ""),
		CephDashboardInsecureTLS: envBool("ATLAS_CEPH_DASHBOARD_INSECURE_TLS", false),
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
