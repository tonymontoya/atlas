package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultAgentStateDir is the working default for ATLAS_AGENT_STATE_DIR:
// a plain local directory keeps development runs visible, and container
// or service deployments override it with a mounted volume path.
const DefaultAgentStateDir = "atlas-agent-state"

// DefaultAgentCollectInterval is the ATLAS_AGENT_COLLECT_INTERVAL
// fallback: a conservative one-minute observation cadence for a
// long-running Agent (ADR-0025).
const DefaultAgentCollectInterval = 60 * time.Second

// DefaultAgentRetryInitial and DefaultAgentRetryMax bound the Agent's
// exponential backoff after a transient failure (network error, HTTP
// 429, or 5xx). Permanent errors (4xx) never retry.
const (
	DefaultAgentRetryInitial = time.Second
	DefaultAgentRetryMax     = 30 * time.Second
)

// AgentConfig is the agent-local configuration for the atlas-agent
// binary (ADR-0025, ADR-0026). It is deliberately separate from the
// control-plane Config: the Agent runs inside a Cluster's trust domain
// and reads its own environment, never Atlas's. Dashboard credentials
// live here only — the Agent never sends them to Atlas.
type AgentConfig struct {
	// AtlasURL is the base URL of the Atlas API the Agent enrolls
	// with and pushes Observation Batches to. It must use https:
	// ingestion requires mutual TLS, so plain HTTP can never work.
	AtlasURL string

	// AtlasRootCAPath optionally points at a PEM CA bundle used to
	// verify the Atlas API's serving certificate, for control planes
	// whose TLS is issued by a private CA. Empty means system roots.
	AtlasRootCAPath string

	// AtlasInsecureTLS skips Atlas certificate verification. Dev-only
	// escape hatch, mirroring the Dashboard provider's flag.
	AtlasInsecureTLS bool

	// EnrollmentCredential is the one-time credential from the
	// Cluster's registration. Required only on first enrollment and
	// on renewal (re-enrollment); otherwise it may stay unset. It
	// comes from the environment or, in container deployments, from
	// the file ATLAS_AGENT_ENROLLMENT_CREDENTIAL_FILE names.
	EnrollmentCredential string

	// StateDir persists the enrolled certificate chain and private
	// key. First run writes it; later runs load it.
	StateDir string

	// CollectInterval is the internal collection ticker. It must be
	// positive; the -once flag covers deterministic one-shot runs.
	CollectInterval time.Duration

	// RetryInitial and RetryMax bound the exponential backoff after
	// transient collection, enrollment, and push failures.
	RetryInitial time.Duration
	RetryMax     time.Duration

	// Dashboard configuration for the Ceph Dashboard read provider
	// running inside the Agent. Credentials are deploy-time
	// configuration and never leave the Agent.
	DashboardURL         string
	DashboardUser        string
	DashboardPassword    string
	DashboardClusterName string
	DashboardInsecureTLS bool
}

// LoadAgent reads the ATLAS_AGENT_* environment and returns a validated
// AgentConfig for the atlas-agent binary. Like Load it fails fast,
// collecting every problem into one error.
func LoadAgent() (AgentConfig, error) {
	cfg := AgentConfig{
		AtlasURL:             env("ATLAS_AGENT_ATLAS_URL", ""),
		AtlasRootCAPath:      env("ATLAS_AGENT_ATLAS_CA_PATH", ""),
		EnrollmentCredential: env("ATLAS_AGENT_ENROLLMENT_CREDENTIAL", ""),
		StateDir:             env("ATLAS_AGENT_STATE_DIR", DefaultAgentStateDir),

		DashboardURL:         env("ATLAS_AGENT_DASHBOARD_URL", ""),
		DashboardUser:        env("ATLAS_AGENT_DASHBOARD_USER", ""),
		DashboardPassword:    env("ATLAS_AGENT_DASHBOARD_PASSWORD", ""),
		DashboardClusterName: env("ATLAS_AGENT_DASHBOARD_CLUSTER_NAME", ""),
	}

	var errs []error

	// The credential file carries the same one-time credential in
	// container deployments (a bootstrap writes it, a mounted
	// secret holds it); inline and file forms are mutually
	// exclusive so the active credential is never ambiguous.
	credentialFile := env("ATLAS_AGENT_ENROLLMENT_CREDENTIAL_FILE", "")
	if credentialFile != "" && cfg.EnrollmentCredential != "" {
		errs = append(errs, errors.New("set only one of ATLAS_AGENT_ENROLLMENT_CREDENTIAL and ATLAS_AGENT_ENROLLMENT_CREDENTIAL_FILE"))
	} else if credentialFile != "" {
		credentialPEM, err := os.ReadFile(credentialFile)
		if err != nil {
			errs = append(errs, fmt.Errorf("read ATLAS_AGENT_ENROLLMENT_CREDENTIAL_FILE %s: %w", credentialFile, err))
		} else {
			cfg.EnrollmentCredential = strings.TrimSpace(string(credentialPEM))
		}
	}

	atlasInsecure, err := envBool("ATLAS_AGENT_ATLAS_INSECURE_TLS", false)
	cfg.AtlasInsecureTLS = atlasInsecure
	if err != nil {
		errs = append(errs, err)
	}

	dashboardInsecure, err := envBool("ATLAS_AGENT_DASHBOARD_INSECURE_TLS", false)
	cfg.DashboardInsecureTLS = dashboardInsecure
	if err != nil {
		errs = append(errs, err)
	}

	cfg.CollectInterval, err = envDuration("ATLAS_AGENT_COLLECT_INTERVAL", DefaultAgentCollectInterval)
	if err != nil {
		errs = append(errs, err)
	} else if cfg.CollectInterval <= 0 {
		errs = append(errs, errors.New("ATLAS_AGENT_COLLECT_INTERVAL must be positive (use the binary's -once flag for one-shot runs)"))
	}

	cfg.RetryInitial, err = envDuration("ATLAS_AGENT_RETRY_INITIAL", DefaultAgentRetryInitial)
	if err != nil {
		errs = append(errs, err)
	} else if cfg.RetryInitial <= 0 {
		errs = append(errs, errors.New("ATLAS_AGENT_RETRY_INITIAL must be positive"))
	}

	cfg.RetryMax, err = envDuration("ATLAS_AGENT_RETRY_MAX", DefaultAgentRetryMax)
	if err != nil {
		errs = append(errs, err)
	} else if cfg.RetryMax <= 0 {
		errs = append(errs, errors.New("ATLAS_AGENT_RETRY_MAX must be positive"))
	} else if cfg.RetryMax < cfg.RetryInitial {
		errs = append(errs, errors.New("ATLAS_AGENT_RETRY_MAX must be at least ATLAS_AGENT_RETRY_INITIAL"))
	}

	if cfg.AtlasURL == "" {
		errs = append(errs, errors.New("ATLAS_AGENT_ATLAS_URL is required (for example https://atlas.example.invalid)"))
	} else if !absoluteURL(cfg.AtlasURL) {
		errs = append(errs, fmt.Errorf("ATLAS_AGENT_ATLAS_URL %q must be an absolute URL with a scheme (for example https://atlas.example.invalid)", cfg.AtlasURL))
	} else if urlScheme(cfg.AtlasURL) != "https" {
		errs = append(errs, fmt.Errorf("ATLAS_AGENT_ATLAS_URL %q must use https: observation ingestion requires mutual TLS", cfg.AtlasURL))
	}

	if cfg.DashboardURL == "" {
		errs = append(errs, errors.New("ATLAS_AGENT_DASHBOARD_URL is required (for example https://mon.example.invalid:8443)"))
	} else if !absoluteURL(cfg.DashboardURL) {
		errs = append(errs, fmt.Errorf("ATLAS_AGENT_DASHBOARD_URL %q must be an absolute URL with a scheme (for example https://mon.example.invalid:8443)", cfg.DashboardURL))
	}
	if cfg.DashboardUser == "" {
		errs = append(errs, errors.New("ATLAS_AGENT_DASHBOARD_USER is required"))
	}
	if cfg.DashboardPassword == "" {
		errs = append(errs, errors.New("ATLAS_AGENT_DASHBOARD_PASSWORD is required"))
	}

	return cfg, errors.Join(errs...)
}

// urlScheme reports a URL's scheme, lowercased; the URL is already
// known to parse.
func urlScheme(raw string) string {
	parsed, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil {
		return ""
	}
	return parsed.Scheme
}
