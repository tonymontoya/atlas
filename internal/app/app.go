package app

import (
	"context"
	"fmt"

	"github.com/tonymontoya/ceph-atlas/internal/agent"
	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/identity"
	"github.com/tonymontoya/ceph-atlas/internal/operations"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/providers/fake"
	"github.com/tonymontoya/ceph-atlas/internal/store"
	"github.com/tonymontoya/ceph-atlas/internal/workflowdispatch"
	"github.com/tonymontoya/ceph-atlas/internal/workflows"
)

type App struct {
	Config               config.Config
	CephProvider         providers.CephReadProvider
	InventorySyncRuns    InventorySyncRunReader
	AlertEvaluationRuns  AlertEvaluationRunReader
	Cases                CaseReader
	CaseWrites           CaseWriter
	ClusterRegistrations ClusterRegistrationStore
	WorkflowReads        WorkflowReader
	WorkflowLifecycle    *workflowdispatch.Lifecycle
	Verifier             *identity.Verifier
	close                func() error
}

type ClusterRegistrationStore interface {
	CreateClusterRegistration(ctx context.Context, input store.ClusterRegistrationInput) (fleet.ClusterRegistration, fleet.EnrollmentCredential, error)
	GetClusterRegistration(ctx context.Context, clusterID int64) (fleet.ClusterRegistration, error)
	DeregisterCluster(ctx context.Context, input store.DeregisterClusterInput) (fleet.ClusterRegistration, error)
}

type InventorySyncRunReader interface {
	ListInventorySyncRuns(ctx context.Context, limit int) ([]store.InventorySyncRun, error)
}

type AlertEvaluationRunReader interface {
	ListAlertEvaluationRuns(ctx context.Context, limit int) ([]store.AlertEvaluationRun, error)
}

type CaseReader interface {
	ListCases(ctx context.Context, limit int) ([]cases.Case, error)
	GetCase(ctx context.Context, id int64) (cases.Case, error)
	ListCaseTimeline(ctx context.Context, caseID int64) ([]cases.TimelineEvent, error)
}

type CaseWriter interface {
	CreateManualCase(ctx context.Context, input store.ManualCaseInput) (cases.Case, error)
	TransitionCase(ctx context.Context, input store.CaseTransitionInput) (cases.Case, error)
	AssignCase(ctx context.Context, input store.CaseAssignmentInput) (cases.Case, error)
	AddCaseNote(ctx context.Context, input store.CaseNoteInput) (cases.CaseNote, error)
	ListCaseNotes(ctx context.Context, caseID int64) ([]cases.CaseNote, error)
}

type WorkflowReader interface {
	GetWorkflowInstance(ctx context.Context, instanceID int64) (store.WorkflowInstance, error)
	ListWorkflowInstancesByCase(ctx context.Context, caseID int64) ([]store.WorkflowInstance, error)
	ListWorkflowJobs(ctx context.Context, instanceID int64) ([]store.WorkflowJob, error)
}

func New(cfg config.Config) *App {
	return &App{
		Config:       cfg,
		CephProvider: fake.New(fake.DefaultFixtureRoot(), cfg.FakeScenario),
	}
}

func NewFromConfig(ctx context.Context, cfg config.Config) (*App, error) {
	// config.Load performs the environment-facing validation. The
	// switches below keep their default arms as backstops for direct
	// construction in tests; canonical values never reach them.
	switch cfg.AgentMode {
	case config.AgentModeDisabled, config.AgentModeFake:
	default:
		return nil, fmt.Errorf("unsupported agent mode %q", cfg.AgentMode)
	}
	verifier, err := verifierFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	switch cfg.ReadSource {
	case config.ReadSourceProvider:
		return &App{Config: cfg, CephProvider: fake.New(fake.DefaultFixtureRoot(), cfg.FakeScenario), Verifier: verifier}, nil
	case config.ReadSourcePostgres:
		db, err := store.OpenPostgres(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, err
		}
		postgresStore := store.NewPostgres(db)
		ops, err := operations.DefaultRegistry()
		if err != nil {
			_ = db.Close()
			return nil, err
		}
		workflowRegistry, err := workflows.DefaultRegistry(ops)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
		// The fake in-process agent loop (ADR-0022) is the only dispatch
		// path; every other mode leaves instances parked — nothing
		// dispatches. The scenario scripts fake agent failures
		// (ATLAS_FAKE_AGENT_SCENARIO); an unknown name fails startup.
		var dispatch *workflowdispatch.Dispatcher
		if cfg.AgentMode == config.AgentModeFake {
			fakeAgent, err := agent.NewFakeWithScenario(ops, cfg.FakeAgentScenario)
			if err != nil {
				_ = db.Close()
				return nil, err
			}
			dispatch = workflowdispatch.New(postgresStore, workflowRegistry, fakeAgent)
		}
		application := &App{
			Config:               cfg,
			CephProvider:         postgresStore,
			InventorySyncRuns:    postgresStore,
			AlertEvaluationRuns:  postgresStore,
			Cases:                postgresStore,
			CaseWrites:           postgresStore,
			ClusterRegistrations: postgresStore,
			WorkflowReads:        postgresStore,
			WorkflowLifecycle:    workflowdispatch.NewLifecycle(postgresStore, workflowRegistry, dispatch),
			Verifier:             verifier,
			close:                db.Close,
		}
		return application, nil
	default:
		return nil, fmt.Errorf("unsupported read source %q", cfg.ReadSource)
	}
}

// verifierFromConfig builds the OIDC bearer-token verifier (ADR-0016).
// config.Load enforces the all-or-none trio rule against the
// environment; the counting here is the backstop for direct struct
// construction.
func verifierFromConfig(cfg config.Config) (*identity.Verifier, error) {
	set := 0
	for _, value := range []string{cfg.OIDCIssuer, cfg.OIDCAudience, cfg.OIDCJWKSURL} {
		if value != "" {
			set++
		}
	}
	switch set {
	case 0:
		return nil, nil
	case 3:
		return identity.NewVerifier(identity.Config{
			Issuer:   cfg.OIDCIssuer,
			Audience: cfg.OIDCAudience,
			JWKSURL:  cfg.OIDCJWKSURL,
		}), nil
	default:
		return nil, fmt.Errorf("identity verification requires all of ATLAS_OIDC_ISSUER, ATLAS_OIDC_AUDIENCE, and ATLAS_OIDC_JWKS_URL")
	}
}

func (a *App) Close() error {
	if a.close == nil {
		return nil
	}
	return a.close()
}
