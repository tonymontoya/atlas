import { describe, expect, it } from "vitest";

import { agentInstallInstructions, credentialExpiresLabel } from "./registrations";

describe("agentInstallInstructions", () => {
  it("points the agent at the credential file env var", () => {
    expect(agentInstallInstructions()).toContain(
      "ATLAS_AGENT_ENROLLMENT_CREDENTIAL_FILE",
    );
  });

  it("requires an https Atlas URL for mutual TLS ingestion", () => {
    expect(agentInstallInstructions()).toContain("ATLAS_AGENT_ATLAS_URL=https://");
  });

  it("keeps Dashboard credentials agent-local with the required env vars", () => {
    const instructions = agentInstallInstructions();
    expect(instructions).toContain("ATLAS_AGENT_DASHBOARD_URL=https://");
    expect(instructions).toContain("ATLAS_AGENT_DASHBOARD_USER");
    expect(instructions).toContain("ATLAS_AGENT_DASHBOARD_PASSWORD");
  });

  it("says what a successful enrollment looks like on the index", () => {
    expect(agentInstallInstructions()).toContain("FSID");
  });
});

describe("credentialExpiresLabel", () => {
  const NOW = Date.parse("2026-09-01T12:00:00Z");

  it("describes days until expiry", () => {
    expect(
      credentialExpiresLabel("2026-09-02T12:00:00Z", NOW),
    ).toBe("expires in 1d");
  });

  it("describes hours until expiry", () => {
    expect(
      credentialExpiresLabel("2026-09-01T15:00:00Z", NOW),
    ).toBe("expires in 3h");
  });

  it("describes minutes until expiry", () => {
    expect(
      credentialExpiresLabel("2026-09-01T12:30:00Z", NOW),
    ).toBe("expires in 30m");
  });

  it("describes seconds until expiry", () => {
    expect(
      credentialExpiresLabel("2026-09-01T12:00:30Z", NOW),
    ).toBe("expires in under a minute");
  });

  it("labels a past expiry as expired", () => {
    expect(credentialExpiresLabel("2026-08-31T12:00:00Z", NOW)).toBe("expired");
  });

  it("labels an unparseable stamp as unknown", () => {
    expect(credentialExpiresLabel("not-a-time", NOW)).toBe("unknown");
  });
});
