import React from "react";
import { Button, TextArea } from "@carbon/react";
import { addCaseNote, type CaseNote, type Operator } from "../../api";
import { formatDate, errorMessage } from "../../format";
import { EmptyState } from "../ui";

export function CaseNotes({
  caseID,
  error,
  loading,
  notes,
  operator,
  token,
  onChanged,
}: {
  caseID: number;
  error: string | null;
  loading: boolean;
  notes: CaseNote[];
  operator: Operator | null;
  token: string | null;
  onChanged: () => void;
}) {
  const [body, setBody] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [noteError, setNoteError] = React.useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (busy || body.trim() === "" || !token) {
      return;
    }
    try {
      setBusy(true);
      setNoteError(null);
      await addCaseNote(caseID, body.trim(), token);
      setBody("");
      onChanged();
    } catch (submitError) {
      setNoteError(errorMessage(submitError));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="atlas-case-subsection" aria-label="Case Notes">
      <h3>
        Notes {!loading && !error ? <span className="atlas-count">{notes.length}</span> : null}
      </h3>
      {loading ? <EmptyState label="Loading Case Notes." /> : null}
      {error ? <EmptyState label={`Case Notes unavailable: ${error}`} /> : null}
      {!loading && !error && notes.length === 0 ? (
        <EmptyState label="No Case Notes recorded." />
      ) : null}
      {!loading && !error && notes.length > 0 ? (
        <ul className="atlas-note-list">
          {notes.map((note) => (
            <li key={note.id} className="atlas-note-row">
              <div className="atlas-note-heading">
                <strong>{note.authorDisplayName}</strong>
                <time dateTime={note.createdAt}>{formatDate(note.createdAt)}</time>
              </div>
              <p>{note.body}</p>
            </li>
          ))}
        </ul>
      ) : null}
      {operator && token ? (
        <form onSubmit={submit}>
          <TextArea
            id="note-body"
            labelText="Add a note"
            rows={2}
            placeholder="Add a note about investigation progress."
            value={body}
            onChange={(event) => setBody(event.target.value)}
          />
          <div className="atlas-action-row">
            <Button type="submit" size="sm" kind="secondary" disabled={busy || body.trim() === ""}>
              {busy ? "Adding…" : "Add note"}
            </Button>
            {noteError ? <p className="atlas-form-error">{noteError}</p> : null}
          </div>
        </form>
      ) : null}
    </section>
  );
}
