// Command atlas-dev-agent-bootstrap prepares one dev-stack bring-up's
// Agent enrollment (#43): it deregisters the previous bring-up's rows
// through the Operator API (the live holder of the fake Dashboard's
// FSID plus any dormant same-name registration), creates a fresh
// Cluster registration through the same API with a dev-issuer bearer
// token, and writes the one-time Enrollment Credential to a file the
// atlas-agent service reads. Dev-only — production enrollment is an
// Operator action (ADR-0025, ADR-0026).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph/dashfake"
)

type options struct {
	apiURL         string
	issuerURL      string
	clusterName    string
	fsid           string
	credentialPath string
	waitTimeout    time.Duration
	interval       time.Duration
	log            *log.Logger
}

func main() {
	opts := options{
		apiURL:         *flag.String("api-url", "http://api:8080", "Atlas API base URL"),
		issuerURL:      *flag.String("issuer-url", "http://dev-issuer:8090", "dev issuer base URL for operator tokens"),
		clusterName:    *flag.String("cluster-name", "dev-agent-cluster", "registration name the bootstrap owns across bring-ups"),
		fsid:           *flag.String("fsid", dashfake.FSID, "FSID whose stale claim a fresh bring-up must release"),
		credentialPath: *flag.String("credential-path", "/agent/enrollment-credential", "file the one-time Enrollment Credential is written to"),
		waitTimeout:    *flag.Duration("wait-timeout", 3*time.Minute, "overall budget for waiting on the API and issuer"),
		interval:       *flag.Duration("interval", 2*time.Second, "poll interval while waiting"),
		log:            log.Default(),
	}
	flag.Parse()

	if err := run(context.Background(), opts); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, opts options) error {
	ctx, cancel := context.WithTimeout(ctx, opts.waitTimeout)
	defer cancel()
	client := &http.Client{}

	if err := waitForHealth(ctx, client, opts); err != nil {
		return err
	}
	token, err := waitForToken(ctx, client, opts)
	if err != nil {
		return err
	}

	// Rerun stability: a previous bring-up's row still holds the fake
	// FSID (deregistered rows release their claim at the fresh
	// enrollment), and a bring-up that died before enrolling leaves a
	// dormant registration behind. Both block or clutter the next
	// bring-up, so retire them through the Operator API first.
	for _, search := range []string{opts.fsid, opts.clusterName} {
		if err := deregisterMatches(ctx, client, opts, token, search); err != nil {
			return err
		}
	}

	credential, err := createRegistration(ctx, client, opts, token)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(opts.credentialPath), 0o700); err != nil {
		return fmt.Errorf("create credential directory: %w", err)
	}
	if err := os.WriteFile(opts.credentialPath, []byte(credential), 0o600); err != nil {
		return fmt.Errorf("write enrollment credential: %w", err)
	}
	opts.log.Printf("bootstrap complete: registration %q ready, credential at %s", opts.clusterName, opts.credentialPath)
	return nil
}

func waitForHealth(ctx context.Context, client *http.Client, opts options) error {
	return poll(ctx, opts, "api health", func() (bool, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.apiURL+"/healthz", nil)
		if err != nil {
			return false, err
		}
		response, err := client.Do(request)
		if err != nil {
			return false, nil
		}
		defer response.Body.Close()
		var health struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(response.Body).Decode(&health); err != nil {
			return false, nil
		}
		return health.Status == "ok", nil
	})
}

func waitForToken(ctx context.Context, client *http.Client, opts options) (string, error) {
	var token string
	err := poll(ctx, opts, "dev issuer token", func() (bool, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, opts.issuerURL+"/token", nil)
		if err != nil {
			return false, err
		}
		response, err := client.Do(request)
		if err != nil {
			return false, nil
		}
		defer response.Body.Close()
		var issued struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(response.Body).Decode(&issued); err != nil || issued.Token == "" {
			return false, nil
		}
		token = issued.Token
		return true, nil
	})
	return token, err
}

func poll(ctx context.Context, opts options, name string, attempt func() (bool, error)) error {
	ticker := time.NewTicker(opts.interval)
	defer ticker.Stop()
	for {
		done, err := attempt()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for %s: %w", name, ctx.Err())
		case <-ticker.C:
		}
	}
}

// deregisterMatches retires every live cluster the index matches for
// one search term. The index excludes deregistered rows already, so
// every match is live and safe to retire.
func deregisterMatches(ctx context.Context, client *http.Client, opts options, token, search string) error {
	matches, err := listClusters(ctx, client, opts, token, search)
	if err != nil {
		return err
	}
	for _, cluster := range matches {
		request, err := authorizedRequest(ctx, http.MethodDelete, fmt.Sprintf("%s/api/v1/clusters/%d", opts.apiURL, cluster.ID), token, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("deregister cluster %d: %w", cluster.ID, err)
		}
		_ = response.Body.Close()
		// 404 and 409 mean another path already retired the row.
		if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNotFound && response.StatusCode != http.StatusConflict {
			return fmt.Errorf("deregister cluster %d: status %d", cluster.ID, response.StatusCode)
		}
		opts.log.Printf("deregistered stale cluster %d (name %q) for bring-up", cluster.ID, cluster.Name)
	}
	return nil
}

type clusterSummary struct {
	ID   int64   `json:"id"`
	FSID *string `json:"fsid"`
	Name string  `json:"name"`
}

func listClusters(ctx context.Context, client *http.Client, opts options, token, search string) ([]clusterSummary, error) {
	request, err := authorizedRequest(ctx, http.MethodGet, opts.apiURL+"/api/v1/clusters?limit=100&q="+search, token, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("list clusters for %q: %w", search, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list clusters for %q: status %d", search, response.StatusCode)
	}
	var index struct {
		Clusters []clusterSummary `json:"clusters"`
	}
	if err := json.NewDecoder(response.Body).Decode(&index); err != nil {
		return nil, fmt.Errorf("decode cluster index: %w", err)
	}
	return index.Clusters, nil
}

func createRegistration(ctx context.Context, client *http.Client, opts options, token string) (string, error) {
	body, err := json.Marshal(map[string]string{
		"name":        opts.clusterName,
		"clusterType": "bare-metal",
	})
	if err != nil {
		return "", err
	}
	request, err := authorizedRequest(ctx, http.MethodPost, opts.apiURL+"/api/v1/clusters", token, body)
	if err != nil {
		return "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("create cluster registration: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		responseBody, _ := io.ReadAll(response.Body)
		return "", fmt.Errorf("create cluster registration: status %d: %s", response.StatusCode, responseBody)
	}
	var created struct {
		EnrollmentCredential struct {
			Token string `json:"token"`
		} `json:"enrollmentCredential"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		return "", fmt.Errorf("decode registration response: %w", err)
	}
	if created.EnrollmentCredential.Token == "" {
		return "", fmt.Errorf("registration response carries no enrollment credential")
	}
	return created.EnrollmentCredential.Token, nil
}

func authorizedRequest(ctx context.Context, method, url, token string, body []byte) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return request, nil
}
