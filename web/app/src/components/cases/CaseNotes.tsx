import React from "react";
import { Button, TextArea } from "@carbon/react";
import { addCaseNote, type CaseNote, type Operator } from "../../api";
import { formatDate } from "../../format";
import { useMutation } from "../../resources";
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
  const add = useMutation((trimmedBody: string) => addCaseNote(caseID, trimmedBody, token ?? ""));

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (body.trim() === "" || !token) {
      return;
    }
    const ok = await add.run(body.trim());
    if (ok) {
      setBody("");
      onChanged();
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
            <Button
              type="submit"
              size="sm"
              kind="secondary"
              disabled={add.busy || body.trim() === ""}
            >
              {add.busy ? "Adding…" : "Add note"}
            </Button>
            {add.error ? <p className="atlas-form-error">{add.error}</p> : null}
          </div>
        </form>
      ) : null}
    </section>
  );
}
