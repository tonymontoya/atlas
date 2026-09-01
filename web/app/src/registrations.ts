// Pure helpers for the cluster registration surfaces. The one-time
// Enrollment Credential can only ever come from the registration
// response, so its display labels and the agent install instructions
// live here as testable strings rather than modal markup.
import { coarseDuration } from "./format";
import type { EnrollmentCredential } from "./api";

// agentInstallInstructions is the install text shown next to a freshly
// minted Enrollment Credential. It mirrors the atlas-agent's
// ATLAS_AGENT_* configuration surface (internal/config.LoadAgent): the
// credential arrives through the file the operator saves, Dashboard
// credentials stay agent-local, and observation ingestion requires
// https for mutual TLS.
export function agentInstallInstructions(): string {
  return [
    "1. Save the one-time Enrollment Credential to a file on the host that will run the Agent.",
    "2. Configure the Agent with its environment:",
    "",
    "ATLAS_AGENT_ATLAS_URL=https://atlas.example.invalid",
    "ATLAS_AGENT_ATLAS_CA_PATH=/path/to/atlas-ca.pem  # only when Atlas TLS uses a private CA",
    "ATLAS_AGENT_DASHBOARD_URL=https://mon.example.invalid:8443",
    "ATLAS_AGENT_DASHBOARD_USER=<read-only dashboard user>",
    "ATLAS_AGENT_DASHBOARD_PASSWORD=<dashboard password>",
    "ATLAS_AGENT_ENROLLMENT_CREDENTIAL_FILE=/path/to/enrollment-credential",
    "",
    "3. Start the Agent (daemon by default; -once for a single collection).",
    "   It enrolls with the credential once, then collects and pushes",
    "   inventory to Atlas over mutual TLS. Dashboard credentials never",
    "   leave the Agent host.",
    "4. Enrollment binds the cluster's FSID on first contact: the row's",
    "   FSID column fills in, then health and inventory start reporting.",
  ].join("\n");
}

// credentialExpiresLabel humanizes a credential's expiry with the
// shared coarse-duration buckets: "expires in 30m", never a countdown.
export function credentialExpiresLabel(
  expiresAt: EnrollmentCredential["expiresAt"],
  now: number = Date.now(),
): string {
  const expires = Date.parse(expiresAt);
  if (Number.isNaN(expires)) {
    return "unknown";
  }
  const until = coarseDuration(expires - now);
  if (until.seconds <= 0) {
    return "expired";
  }
  if (until.seconds < 60) {
    return "expires in under a minute";
  }
  if (until.minutes < 60) {
    return `expires in ${until.minutes}m`;
  }
  if (until.hours < 24) {
    return `expires in ${until.hours}h`;
  }
  return `expires in ${until.days}d`;
}
