package app

import (
	"context"
	"fmt"

	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/identity"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/providers/fake"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

type App struct {
	Config              config.Config
	CephProvider        providers.CephReadProvider
	InventorySyncRuns   InventorySyncRunReader
	AlertEvaluationRuns AlertEvaluationRunReader
	Cases               CaseReader
	CaseWrites          CaseWriter
	Verifier            *identity.Verifier
	close               func() error
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

func New(cfg config.Config) *App {
	return &App{
		Config:       cfg,
		CephProvider: fake.New(fake.DefaultFixtureRoot(), cfg.FakeScenario),
	}
}

func NewFromConfig(ctx context.Context, cfg config.Config) (*App, error) {
	verifier, err := verifierFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	switch cfg.ReadSource {
	case "", "provider":
		return &App{Config: cfg, CephProvider: fake.New(fake.DefaultFixtureRoot(), cfg.FakeScenario), Verifier: verifier}, nil
	case "postgres":
		db, err := store.OpenPostgres(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, err
		}
		postgresStore := store.NewPostgres(db)
		return &App{
			Config:              cfg,
			CephProvider:        postgresStore,
			InventorySyncRuns:   postgresStore,
			AlertEvaluationRuns: postgresStore,
			Cases:               postgresStore,
			CaseWrites:          postgresStore,
			Verifier:            verifier,
			close:               db.Close,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported read source %q", cfg.ReadSource)
	}
}

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
