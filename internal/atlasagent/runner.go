package atlasagent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
)

// logger is the Runner's logging seam; *log.Logger satisfies it.
type logger interface {
	Printf(format string, args ...any)
}

// Options configures the Runner. Config must already be validated
// (config.LoadAgent); Provider is the Dashboard read provider running
// inside the Agent.
type Options struct {
	Config   config.AgentConfig
	Provider providers.CephReadProvider
	Log      logger // nil discards logs
}

// Runner drives the Agent's runtime (ADR-0025, ADR-0026): enroll once
// and persist the certificate, then collect complete inventory batches
// and push them over mutual TLS, retrying transient failures with
// backoff. Permanent failures surface to the operator. The Runner has
// no inbound surface at all — nothing dispatches commands to it.
type Runner struct {
	cfg      config.AgentConfig
	provider providers.CephReadProvider
	state    StateStore
	log      logger
	tls      TLSOptions
	now      func() time.Time

	// push is built by EnsureEnrolled from the loaded or freshly
	// issued enrollment.
	push *PushClient
}

func NewRunner(opts Options) *Runner {
	var sink logger = discardLogger{}
	if opts.Log != nil {
		sink = opts.Log
	}
	return &Runner{
		cfg:      opts.Config,
		provider: opts.Provider,
		state:    StateStore{Dir: opts.Config.StateDir},
		log:      sink,
		tls: TLSOptions{
			RootCAPath:         opts.Config.AtlasRootCAPath,
			InsecureSkipVerify: opts.Config.AtlasInsecureTLS,
		},
		now: time.Now,
	}
}

type discardLogger struct{}

func (discardLogger) Printf(string, ...any) {}

// RunOnce runs one full pass — ensure enrolled, collect, push — for
// the binary's one-shot mode.
func (r *Runner) RunOnce(ctx context.Context) error {
	if err := r.EnsureEnrolled(ctx); err != nil {
		return err
	}
	return r.RunCycle(ctx)
}

// RunDaemon enrolls once at startup, then collects and pushes on the
// configured interval until the context ends. It returns nil on clean
// shutdown and the first permanent error otherwise: a rejected or
// conflicting certificate needs an operator, not a retry.
func (r *Runner) RunDaemon(ctx context.Context) error {
	if err := r.EnsureEnrolled(ctx); err != nil {
		return err
	}
	r.log.Printf("atlas-agent collecting every %s until shutdown", r.cfg.CollectInterval)

	if err := r.RunCycle(ctx); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	ticker := time.NewTicker(r.cfg.CollectInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := r.RunCycle(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

// EnsureEnrolled leaves the Runner holding a push client for the
// stored enrollment, enrolling (or re-enrolling an expired identity)
// when it must.
func (r *Runner) EnsureEnrolled(ctx context.Context) error {
	enrollment, err := r.state.Load()
	switch {
	case err == nil:
		if enrollment.Leaf.NotAfter.After(r.now()) {
			return r.adoptEnrollment(enrollment)
		}
		r.log.Printf("stored certificate expired at %s", enrollment.Leaf.NotAfter.Format(time.RFC3339))
		if r.cfg.EnrollmentCredential == "" {
			return &permanentError{err: fmt.Errorf(
				"stored enrollment certificate in %s expired at %s: re-register the cluster in Atlas and set ATLAS_AGENT_ENROLLMENT_CREDENTIAL to re-enroll (ADR-0026)",
				r.cfg.StateDir, enrollment.Leaf.NotAfter.Format(time.RFC3339))}
		}
	case errors.Is(err, ErrNoEnrollment):
		if r.cfg.EnrollmentCredential == "" {
			return &permanentError{err: fmt.Errorf(
				"no stored enrollment in %s and ATLAS_AGENT_ENROLLMENT_CREDENTIAL is not set: register the cluster in Atlas and set the credential for first enrollment",
				r.cfg.StateDir)}
		}
	default:
		// Corrupt or unreadable state is an operator problem.
		return &permanentError{err: err}
	}
	return r.enroll(ctx)
}

// enroll performs the enrollment handshake with backoff: read the
// FSID from the Dashboard, generate a fresh key and CSR, exchange the
// one-time credential for a certificate, persist both. Nothing is
// written until Atlas accepts the handshake.
func (r *Runner) enroll(ctx context.Context) error {
	retrier := r.retrier()
	return retrier.Do(ctx, func(err error, delay time.Duration) {
		r.log.Printf("enrollment attempt failed (%v); retrying in %s", err, delay)
	}, func() error {
		identity, err := r.provider.ClusterIdentity(ctx)
		if err != nil {
			return fmt.Errorf("read cluster identity from dashboard: %w", err)
		}
		key, err := NewKeyPair()
		if err != nil {
			return err
		}
		csrPEM, err := NewCSRPEM(key)
		if err != nil {
			return err
		}
		client, err := NewEnrollClient(r.cfg.AtlasURL, r.tls)
		if err != nil {
			return &permanentError{err: err}
		}
		enrollment, receipt, err := client.Enroll(ctx, EnrollRequest{
			CredentialToken: r.cfg.EnrollmentCredential,
			FSID:            identity.FSID,
			CSRPEM:          csrPEM,
		}, key)
		if err != nil {
			return err
		}
		if err := r.state.Save(enrollment); err != nil {
			return &permanentError{err: err}
		}
		r.log.Printf("enrolled cluster %q (id %d, fsid %s); certificate valid until %s",
			receipt.ClusterName, receipt.ClusterID, receipt.FSID, enrollment.Leaf.NotAfter.Format(time.RFC3339))
		return r.adoptEnrollment(&enrollment)
	})
}

// adoptEnrollment builds the mutual-TLS push client for an enrollment.
func (r *Runner) adoptEnrollment(enrollment *Enrollment) error {
	push, err := NewPushClient(r.cfg.AtlasURL, enrollment, r.tls)
	if err != nil {
		return &permanentError{err: err}
	}
	r.push = push
	return nil
}

// RunCycle collects one complete batch and pushes it, retrying
// transient failures with backoff. Every attempt collects a fresh,
// consistent batch: a partial batch is never pushed, and a batch is
// never mixed across collection cycles.
func (r *Runner) RunCycle(ctx context.Context) error {
	if r.push == nil {
		return &permanentError{err: errors.New("runner is not enrolled yet")}
	}
	retrier := r.retrier()
	return retrier.Do(ctx, func(err error, delay time.Duration) {
		r.log.Printf("collect-and-push attempt failed (%v); retrying in %s", err, delay)
	}, func() error {
		batch, err := Collect(ctx, r.provider, r.now().UTC())
		if err != nil {
			return err
		}
		receipt, err := r.push.Push(ctx, batch)
		if err != nil {
			return err
		}
		r.log.Printf("pushed observation batch cluster_id=%d snapshot_id=%d osds=%d hosts=%d devices=%d",
			receipt.ClusterID, receipt.SnapshotID, len(batch.OSDs), len(batch.Hosts), len(batch.Devices))
		return nil
	})
}

func (r *Runner) retrier() Retrier {
	return Retrier{Initial: r.cfg.RetryInitial, Max: r.cfg.RetryMax}
}
