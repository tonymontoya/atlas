package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// ProviderMode selects which Ceph read provider adapters construct. It is
// the typed form of ATLAS_PROVIDER_MODE; Load canonicalizes and validates
// it before any caller sees it.
type ProviderMode string

const (
	ProviderModeFake ProviderMode = "fake"
	ProviderModeCeph ProviderMode = "ceph"
)

// ReadSource selects where the API serves inventory reads from. It is the
// typed form of ATLAS_READ_SOURCE; the empty env value canonicalizes to
// ReadSourceProvider in Load.
type ReadSource string

const (
	ReadSourceProvider ReadSource = "provider"
	ReadSourcePostgres ReadSource = "postgres"
)

// AgentMode selects whether Workflow Instances dispatch and which agent
// adapter drives them. It is the typed form of ATLAS_AGENT_MODE; the empty
// env value canonicalizes to AgentModeDisabled in Load.
type AgentMode string

const (
	AgentModeDisabled AgentMode = "disabled"
	AgentModeFake     AgentMode = "fake"
)

// AlertSource selects where alert evaluation reads alerts from. It is
// the typed form of ATLAS_ALERT_SOURCE; the empty env value
// canonicalizes to AlertSourceFake in Load so local development and CI
// stay fake-first (ADR-0027).
type AlertSource string

const (
	AlertSourceFake       AlertSource = "fake"
	AlertSourcePrometheus AlertSource = "prometheus"
)

type Config struct {
	HTTPAddr          string
	DatabaseURL       string
	ProviderMode      ProviderMode
	FakeScenario      string
	FakeAlertScenario string
	FakeAgentScenario string
	ReadSource        ReadSource
	AgentMode         AgentMode
	AlertSource       AlertSource
	AlertEvalInterval time.Duration
	OIDCIssuer        string
	OIDCAudience      string
	OIDCJWKSURL       string

	CephDashboardURL         string
	CephDashboardUser        string
	CephDashboardPassword    string
	CephClusterName          string
	CephDashboardInsecureTLS bool

	PrometheusURL         string
	PrometheusBearerToken string
	PrometheusInsecureTLS bool

	EnrollmentCACertPath string
	EnrollmentCAKeyPath  string
}

// Load reads the ATLAS_* environment and returns a validated Config.
// Validation fails fast, before any adapter is constructed: every problem
// is collected into one error naming the environment variables the
// operator actually set. Config is a pure leaf — it must not import
// anything from internal/.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:          env("ATLAS_HTTP_ADDR", ":8080"),
		DatabaseURL:       env("ATLAS_DATABASE_URL", "postgres://atlas:atlas_dev@127.0.0.1:15432/atlas?sslmode=disable"),
		ProviderMode:      ProviderMode(env("ATLAS_PROVIDER_MODE", string(ProviderModeFake))),
		FakeScenario:      env("ATLAS_FAKE_SCENARIO", "reef-healthy-baremetal"),
		FakeAlertScenario: env("ATLAS_FAKE_ALERT_SCENARIO", "osd-down-alert"),
		FakeAgentScenario: env("ATLAS_FAKE_AGENT_SCENARIO", ""),
		ReadSource:        ReadSource(env("ATLAS_READ_SOURCE", string(ReadSourceProvider))),
		AgentMode:         AgentMode(env("ATLAS_AGENT_MODE", string(AgentModeDisabled))),
		AlertSource:       AlertSource(env("ATLAS_ALERT_SOURCE", string(AlertSourceFake))),
		OIDCIssuer:        env("ATLAS_OIDC_ISSUER", ""),
		OIDCAudience:      env("ATLAS_OIDC_AUDIENCE", ""),
		OIDCJWKSURL:       env("ATLAS_OIDC_JWKS_URL", ""),

		CephDashboardURL:      env("ATLAS_CEPH_DASHBOARD_URL", ""),
		CephDashboardUser:     env("ATLAS_CEPH_DASHBOARD_USER", ""),
		CephDashboardPassword: env("ATLAS_CEPH_DASHBOARD_PASSWORD", ""),
		CephClusterName:       env("ATLAS_CEPH_CLUSTER_NAME", ""),

		PrometheusURL:         env("ATLAS_PROMETHEUS_URL", ""),
		PrometheusBearerToken: env("ATLAS_PROMETHEUS_BEARER_TOKEN", ""),

		EnrollmentCACertPath: env("ATLAS_ENROLLMENT_CA_CERT_PATH", ""),
		EnrollmentCAKeyPath:  env("ATLAS_ENROLLMENT_CA_KEY_PATH", ""),
	}

	var errs []error

	insecureTLS, err := envBool("ATLAS_CEPH_DASHBOARD_INSECURE_TLS", false)
	cfg.CephDashboardInsecureTLS = insecureTLS
	if err != nil {
		errs = append(errs, err)
	}

	prometheusInsecureTLS, err := envBool("ATLAS_PROMETHEUS_INSECURE_TLS", false)
	cfg.PrometheusInsecureTLS = prometheusInsecureTLS
	if err != nil {
		errs = append(errs, err)
	}

	alertEvalInterval, err := envDuration("ATLAS_ALERT_EVAL_INTERVAL", 0)
	cfg.AlertEvalInterval = alertEvalInterval
	if err != nil {
		errs = append(errs, err)
	}

	switch cfg.ProviderMode {
	case ProviderModeFake, ProviderModeCeph:
	default:
		errs = append(errs, fmt.Errorf("unsupported ATLAS_PROVIDER_MODE %q (supported: fake, ceph)", cfg.ProviderMode))
	}

	switch cfg.ReadSource {
	case ReadSourceProvider, ReadSourcePostgres:
	default:
		errs = append(errs, fmt.Errorf("unsupported ATLAS_READ_SOURCE %q (supported: provider, postgres)", cfg.ReadSource))
	}

	switch cfg.AgentMode {
	case AgentModeDisabled, AgentModeFake:
	default:
		errs = append(errs, fmt.Errorf("unsupported ATLAS_AGENT_MODE %q (supported: disabled, fake)", cfg.AgentMode))
	}

	switch cfg.AlertSource {
	case AlertSourceFake, AlertSourcePrometheus:
	default:
		errs = append(errs, fmt.Errorf("unsupported ATLAS_ALERT_SOURCE %q (supported: fake, prometheus)", cfg.AlertSource))
	}

	set := 0
	for _, value := range []string{cfg.OIDCIssuer, cfg.OIDCAudience, cfg.OIDCJWKSURL} {
		if value != "" {
			set++
		}
	}
	if set != 0 && set != 3 {
		errs = append(errs, errors.New("identity verification requires all of ATLAS_OIDC_ISSUER, ATLAS_OIDC_AUDIENCE, and ATLAS_OIDC_JWKS_URL"))
	}

	// The enrollment CA key is control-plane configuration (ADR-0026):
	// optional everywhere, but never half-configured.
	set = 0
	for _, value := range []string{cfg.EnrollmentCACertPath, cfg.EnrollmentCAKeyPath} {
		if value != "" {
			set++
		}
	}
	if set != 0 && set != 2 {
		errs = append(errs, errors.New("agent enrollment requires both ATLAS_ENROLLMENT_CA_CERT_PATH and ATLAS_ENROLLMENT_CA_KEY_PATH"))
	}

	if cfg.ProviderMode == ProviderModeCeph {
		errs = appendCephDashboardChecks(errs, cfg)
	}

	if cfg.AlertSource == AlertSourcePrometheus {
		errs = appendPrometheusAlertChecks(errs, cfg)
	}

	return cfg, errors.Join(errs...)
}

// appendCephDashboardChecks enforces the ATLAS_PROVIDER_MODE=ceph
// cross-field contract: claiming ceph mode requires a Dashboard URL with
// a scheme and read-only credentials, in every binary.
func appendCephDashboardChecks(errs []error, cfg Config) []error {
	if cfg.CephDashboardURL == "" {
		errs = append(errs, errors.New("ATLAS_PROVIDER_MODE=ceph requires ATLAS_CEPH_DASHBOARD_URL"))
	} else if !absoluteURL(cfg.CephDashboardURL) {
		errs = append(errs, fmt.Errorf("ATLAS_CEPH_DASHBOARD_URL %q must be an absolute URL with a scheme (for example https://mon.example.invalid:8443)", cfg.CephDashboardURL))
	}
	if cfg.CephDashboardUser == "" {
		errs = append(errs, errors.New("ATLAS_PROVIDER_MODE=ceph requires ATLAS_CEPH_DASHBOARD_USER"))
	}
	if cfg.CephDashboardPassword == "" {
		errs = append(errs, errors.New("ATLAS_PROVIDER_MODE=ceph requires ATLAS_CEPH_DASHBOARD_PASSWORD"))
	}
	return errs
}

// appendPrometheusAlertChecks enforces the ATLAS_ALERT_SOURCE=prometheus
// cross-field contract: claiming a real Prometheus alert source requires
// an absolute URL, in every binary. The bearer token and insecure-TLS
// flag are optional because lab Prometheus deployments run without
// authentication and with self-signed certificates.
func appendPrometheusAlertChecks(errs []error, cfg Config) []error {
	if cfg.PrometheusURL == "" {
		errs = append(errs, errors.New("ATLAS_ALERT_SOURCE=prometheus requires ATLAS_PROMETHEUS_URL"))
	} else if !absoluteURL(cfg.PrometheusURL) {
		errs = append(errs, fmt.Errorf("ATLAS_PROMETHEUS_URL %q must be an absolute URL with a scheme (for example http://prometheus.example.invalid:9090)", cfg.PrometheusURL))
	}
	return errs
}

// absoluteURL reports whether raw parses as a URL carrying both a scheme
// and a host. Scheme-less strings such as "mon.example:8443" parse as an
// opaque path and are rejected.
func absoluteURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimRight(raw, "/"))
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback, fmt.Errorf("invalid boolean for %s: %q (want true or false)", key, value)
	}
	return parsed, nil
}

// envDuration reads a Go duration. The fallback keeps one-shot commands
// one-shot: a zero interval means "evaluate once".
func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < 0 {
		return fallback, fmt.Errorf("invalid duration for %s: %q (want for example 30s or 5m)", key, value)
	}
	return parsed, nil
}
