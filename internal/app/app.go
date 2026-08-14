package app

import (
	"context"
	"fmt"

	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/providers/fake"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

type App struct {
	Config            config.Config
	CephProvider      providers.CephReadProvider
	InventorySyncRuns InventorySyncRunReader
	Cases             CaseReader
	close             func() error
}

type InventorySyncRunReader interface {
	ListInventorySyncRuns(ctx context.Context, limit int) ([]store.InventorySyncRun, error)
}

type CaseReader interface {
	ListCases(ctx context.Context, limit int) ([]cases.Case, error)
	GetCase(ctx context.Context, id int64) (cases.Case, error)
	ListCaseTimeline(ctx context.Context, caseID int64) ([]cases.TimelineEvent, error)
}

func New(cfg config.Config) *App {
	return &App{
		Config:       cfg,
		CephProvider: fake.New(fake.DefaultFixtureRoot(), cfg.FakeScenario),
	}
}

func NewFromConfig(ctx context.Context, cfg config.Config) (*App, error) {
	switch cfg.ReadSource {
	case "", "provider":
		return New(cfg), nil
	case "postgres":
		db, err := store.OpenPostgres(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, err
		}
		postgresStore := store.NewPostgres(db)
		return &App{
			Config:            cfg,
			CephProvider:      postgresStore,
			InventorySyncRuns: postgresStore,
			Cases:             postgresStore,
			close:             db.Close,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported read source %q", cfg.ReadSource)
	}
}

func (a *App) Close() error {
	if a.close == nil {
		return nil
	}
	return a.close()
}
