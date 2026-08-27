import React from "react";
import {
  Button,
  Column,
  Grid,
  Select,
  SelectItem,
  TextArea,
  TextInput,
} from "@carbon/react";
import { createCase, type CaseRecord } from "../../api";
import { errorMessage } from "../../format";

const SEVERITIES: CaseRecord["severity"][] = ["info", "low", "medium", "high", "critical"];

export function CaseComposePanel({
  token,
  defaultClusterFsid,
  onCreated,
}: {
  token: string;
  defaultClusterFsid?: string;
  onCreated: (created: CaseRecord) => void;
}) {
  const [title, setTitle] = React.useState("");
  const [summary, setSummary] = React.useState("");
  const [severity, setSeverity] = React.useState<CaseRecord["severity"]>("medium");
  const [clusterFsid, setClusterFsid] = React.useState(defaultClusterFsid ?? "");
  const [busy, setBusy] = React.useState(false);
  const [composeError, setComposeError] = React.useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (busy || title.trim() === "" || summary.trim() === "") {
      return;
    }
    try {
      setBusy(true);
      setComposeError(null);
      const created = await createCase(
        {
          title: title.trim(),
          summary: summary.trim(),
          severity,
          clusterFsid: clusterFsid.trim() === "" ? undefined : clusterFsid.trim(),
        },
        token,
      );
      setTitle("");
      setSummary("");
      setSeverity("medium");
      setClusterFsid(defaultClusterFsid ?? "");
      onCreated(created);
    } catch (submitError) {
      setComposeError(errorMessage(submitError));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="atlas-panel" aria-label="Create a manual case">
      <h2 className="atlas-panel-heading">New Manual Case</h2>
      <form onSubmit={submit}>
        <Grid fullWidth>
          <Column sm={4} md={4} lg={6}>
            <TextInput
              id="case-title"
              labelText="Title"
              placeholder="Manual review of slow OSD warnings"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
            />
          </Column>
          <Column sm={4} md={4} lg={4}>
            <Select
              id="case-severity"
              labelText="Severity"
              value={severity}
              onChange={(event) => setSeverity(event.target.value as CaseRecord["severity"])}
            >
              {SEVERITIES.map((value) => (
                <SelectItem key={value} value={value} text={value} />
              ))}
            </Select>
          </Column>
          <Column sm={4} md={8} lg={10}>
            <TextArea
              id="case-summary"
              labelText="Summary"
              rows={2}
              placeholder="What the operator observed and why this needs a case."
              value={summary}
              onChange={(event) => setSummary(event.target.value)}
            />
          </Column>
          <Column sm={4} md={8} lg={10}>
            <TextInput
              id="case-cluster-fsid"
              labelText="Cluster FSID (optional)"
              placeholder="00000000-0000-4000-8000-000000000101"
              value={clusterFsid}
              onChange={(event) => setClusterFsid(event.target.value)}
            />
          </Column>
        </Grid>
        <div className="atlas-action-row">
          <Button type="submit" disabled={busy || title.trim() === "" || summary.trim() === ""}>
            {busy ? "Creating…" : "Create case"}
          </Button>
          {composeError ? <p className="atlas-form-error">{composeError}</p> : null}
        </div>
      </form>
    </section>
  );
}
