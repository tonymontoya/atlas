import React from "react";
import { Button, TextInput } from "@carbon/react";
import {
  approveWorkflowGate,
  attachCaseWorkflow,
  completeWorkflowTask,
  type WorkflowInstanceRecord,
  type WorkflowJobRecord,
} from "../../api";
import { formatDate, errorMessage } from "../../format";
import { toneForJobState, toneForWorkflowState } from "../../tones";
import { EmptyState, StatusTag } from "../ui";

export function CaseWorkflows({
  caseID,
  error,
  loading,
  workflows,
  workflowJobs,
  canAttach,
  token,
  onChanged,
}: {
  caseID: number;
  error: string | null;
  loading: boolean;
  workflows: WorkflowInstanceRecord[];
  workflowJobs: Record<number, WorkflowJobRecord[]>;
  canAttach: boolean;
  token: string | null;
  onChanged: () => void;
}) {
  const [workflowID, setWorkflowID] = React.useState("replace-osd");
  const [workflowVersion, setWorkflowVersion] = React.useState("1");
  const [busy, setBusy] = React.useState(false);
  const [attachError, setAttachError] = React.useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    const version = Number.parseInt(workflowVersion, 10);
    if (busy || workflowID.trim() === "" || !Number.isInteger(version) || version < 1 || !token) {
      return;
    }
    try {
      setBusy(true);
      setAttachError(null);
      await attachCaseWorkflow(caseID, workflowID.trim(), version, token);
      onChanged();
    } catch (thrown) {
      setAttachError(errorMessage(thrown));
    } finally {
      setBusy(false);
    }
  }

  const formReady = workflowID.trim() !== "" && Number.parseInt(workflowVersion, 10) >= 1;

  return (
    <section className="atlas-case-subsection" aria-label="Workflows">
      <h3>
        Workflows{" "}
        {!loading && !error ? <span className="atlas-count">{workflows.length}</span> : null}
      </h3>
      {loading ? <EmptyState label="Loading Workflow Instances." /> : null}
      {error ? <EmptyState label={`Workflow Instances unavailable: ${error}`} /> : null}
      {!loading && !error && workflows.length === 0 ? (
        <EmptyState label="No Workflow Instances attached." />
      ) : null}
      {!loading && !error && workflows.length > 0 ? (
        <ul className="atlas-workflow-list">
          {workflows.map((instance) => (
            <li key={instance.id} className="atlas-workflow-row">
              <div className="atlas-workflow-heading">
                <strong>
                  {instance.workflowId} v{instance.workflowVersion}
                </strong>
                <StatusTag
                  label={instance.state.replace(/_/g, " ")}
                  tone={toneForWorkflowState(instance.state)}
                />
              </div>
              <p className="atlas-subtle">
                #{instance.id}
                {instance.currentStep ? ` · paused at ${instance.currentStep}` : ""}
                {` · updated ${formatDate(instance.updatedAt)}`}
              </p>
              {workflowJobs[instance.id]?.length ? (
                <ul className="atlas-job-list">
                  {workflowJobs[instance.id].map((job) => (
                    <li key={job.id} className="atlas-job-row">
                      <span>{job.stepId}</span>
                      <span>{job.operationType}</span>
                      <StatusTag label={job.state} tone={toneForJobState(job.state)} />
                      {job.maxAttempts > 1 ? (
                        <span className="atlas-subtle">{`attempt ${job.attempt}/${job.maxAttempts}`}</span>
                      ) : null}
                    </li>
                  ))}
                </ul>
              ) : null}
              {instance.state === "waiting_for_approval" && instance.currentStep && token ? (
                <WorkflowApproveForm
                  gateID={instance.currentStep}
                  instanceID={instance.id}
                  token={token}
                  onChanged={onChanged}
                />
              ) : null}
              {instance.state === "waiting_for_operator" && instance.currentStep && token ? (
                <WorkflowResumeForm
                  taskID={instance.currentStep}
                  instanceID={instance.id}
                  token={token}
                  onChanged={onChanged}
                />
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}
      {canAttach ? (
        <form onSubmit={submit} className="atlas-inline-form">
          <TextInput
            id="workflow-id"
            hideLabel
            labelText="Workflow id"
            placeholder="Workflow id, e.g. replace-osd"
            value={workflowID}
            onChange={(event) => setWorkflowID(event.target.value)}
          />
          <TextInput
            id="workflow-version"
            hideLabel
            labelText="Version"
            placeholder="Version"
            value={workflowVersion}
            onChange={(event) => setWorkflowVersion(event.target.value)}
          />
          <Button type="submit" size="sm" kind="secondary" disabled={busy || !formReady}>
            {busy ? "Attaching…" : "Attach workflow"}
          </Button>
          {attachError ? <p className="atlas-form-error">{attachError}</p> : null}
        </form>
      ) : null}
    </section>
  );
}

function WorkflowApproveForm({
  gateID,
  instanceID,
  token,
  onChanged,
}: {
  gateID: string;
  instanceID: number;
  token: string;
  onChanged: () => void;
}) {
  const [reason, setReason] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [approveError, setApproveError] = React.useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (busy) {
      return;
    }
    try {
      setBusy(true);
      setApproveError(null);
      await approveWorkflowGate(instanceID, gateID, reason.trim(), token);
      setReason("");
      onChanged();
    } catch (thrown) {
      setApproveError(errorMessage(thrown));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="atlas-inline-form">
      <TextInput
        id={`approve-reason-${instanceID}`}
        hideLabel
        labelText="Approval reason"
        placeholder="Reason (optional)"
        value={reason}
        onChange={(event) => setReason(event.target.value)}
      />
      <Button type="submit" size="sm">
        {busy ? "Approving…" : `Approve ${gateID}`}
      </Button>
      {approveError ? <p className="atlas-form-error">{approveError}</p> : null}
    </form>
  );
}

function WorkflowResumeForm({
  taskID,
  instanceID,
  token,
  onChanged,
}: {
  taskID: string;
  instanceID: number;
  token: string;
  onChanged: () => void;
}) {
  const [note, setNote] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [resumeError, setResumeError] = React.useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (busy) {
      return;
    }
    try {
      setBusy(true);
      setResumeError(null);
      await completeWorkflowTask(instanceID, taskID, note.trim(), token);
      setNote("");
      onChanged();
    } catch (thrown) {
      setResumeError(errorMessage(thrown));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="atlas-inline-form">
      <TextInput
        id={`task-note-${instanceID}`}
        hideLabel
        labelText="Task completion note"
        placeholder="Note (optional)"
        value={note}
        onChange={(event) => setNote(event.target.value)}
      />
      <Button type="submit" size="sm">
        {busy ? "Resuming…" : `Done: ${taskID}`}
      </Button>
      {resumeError ? <p className="atlas-form-error">{resumeError}</p> : null}
    </form>
  );
}
