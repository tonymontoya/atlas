import React from "react";
import { Button, TextInput } from "@carbon/react";
import type { Operator } from "../api";
import { errorMessage } from "../format";

// OperatorPanel carries the paste-a-bearer-token sign-in model (ADR-0016):
// manual Case writes need an OIDC bearer token, verified through /api/v1/me.
export function OperatorPanel({
  operator,
  onSignIn,
  onSignOut,
}: {
  operator: Operator | null;
  onSignIn: (token: string) => Promise<void>;
  onSignOut: () => void;
}) {
  const [tokenInput, setTokenInput] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [signInError, setSignInError] = React.useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    const trimmed = tokenInput.trim();
    if (!trimmed || busy) {
      return;
    }
    try {
      setBusy(true);
      setSignInError(null);
      await onSignIn(trimmed);
      setTokenInput("");
    } catch (submitError) {
      setSignInError(errorMessage(submitError));
    } finally {
      setBusy(false);
    }
  }

  if (operator) {
    return (
      <section className="atlas-operator" aria-label="Operator session">
        <p>
          Signed in as <strong>{operator.displayName}</strong>{" "}
          <span className="atlas-subtle">({operator.subject})</span>. Manual case writes are
          enabled.
        </p>
        <Button size="sm" kind="secondary" onClick={onSignOut}>
          Sign out
        </Button>
      </section>
    );
  }

  return (
    <section className="atlas-operator" aria-label="Operator session">
      <form onSubmit={submit}>
        <TextInput
          id="operator-token"
          type="password"
          labelText="Operator token"
          helperText="Paste a bearer token to enable manual case writes. Local development: request one from the dev issuer (POST /token on the dev issuer port)."
          placeholder="eyJhbGciOiJSUzI1NiIs..."
          value={tokenInput}
          onChange={(event) => setTokenInput(event.target.value)}
          autoComplete="off"
        />
        <Button
          type="submit"
          size="sm"
          disabled={busy || tokenInput.trim() === ""}
        >
          {busy ? "Verifying…" : "Sign in"}
        </Button>
      </form>
      {signInError ? <p className="atlas-form-error">{signInError}</p> : null}
    </section>
  );
}
