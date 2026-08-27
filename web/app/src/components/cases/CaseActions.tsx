import React from "react";
import { Button, TextInput } from "@carbon/react";
import { assignCase, transitionCase, type CaseRecord } from "../../api";
import { availableCaseActions } from "../../caseActions";
import { useMutation } from "../../resources";

export function CaseActions({
  detail,
  token,
  onChanged,
}: {
  detail: CaseRecord;
  token: string;
  onChanged: () => void;
}) {
  const [assignee, setAssignee] = React.useState("");
  const [assigneeName, setAssigneeName] = React.useState("");
  const mutation = useMutation((action: () => Promise<unknown>) => action());

  async function run(action: () => Promise<unknown>) {
    const ok = await mutation.run(action);
    if (ok) {
      onChanged();
    }
  }

  const actions = availableCaseActions(detail.status);
  const assignFormReady = assignee.trim() !== "" && assigneeName.trim() !== "";

  return (
    <div className="atlas-case-actions">
      <div className="atlas-action-row">
        {actions.canTriage ? (
          <Button
            size="sm"
            disabled={mutation.busy}
            onClick={() => run(() => transitionCase(detail.id, "triaged", token))}
          >
            Triage
          </Button>
        ) : null}
        {actions.canClose ? (
          <Button
            size="sm"
            kind="danger"
            disabled={mutation.busy}
            onClick={() => run(() => transitionCase(detail.id, "closed", token))}
          >
            Close case
          </Button>
        ) : null}
        {actions.isClosed ? (
          <p className="atlas-subtle">
            Closed. Reopening means creating a new case for a recurring condition.
          </p>
        ) : (
          <div className="atlas-inline-form">
            <TextInput
              id="assignee-subject"
              hideLabel
              labelText="Assignee subject"
              placeholder="Assignee subject"
              value={assignee}
              onChange={(event) => setAssignee(event.target.value)}
            />
            <TextInput
              id="assignee-name"
              hideLabel
              labelText="Assignee display name"
              placeholder="Assignee display name"
              value={assigneeName}
              onChange={(event) => setAssigneeName(event.target.value)}
            />
            <Button
              size="sm"
              kind="secondary"
              disabled={mutation.busy || !assignFormReady}
              onClick={() =>
                run(async () => {
                  await assignCase(detail.id, assignee.trim(), assigneeName.trim(), token);
                  setAssignee("");
                  setAssigneeName("");
                })
              }
            >
              Assign
            </Button>
            <Button
              size="sm"
              kind="ghost"
              disabled={mutation.busy || !detail.assignee}
              onClick={() => run(() => assignCase(detail.id, "", "", token))}
            >
              Unassign
            </Button>
          </div>
        )}
      </div>
      {mutation.error ? <p className="atlas-form-error">{mutation.error}</p> : null}
    </div>
  );
}
