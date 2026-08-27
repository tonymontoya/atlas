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
import { useMutation } from "../../resources";

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
  // run resolves a boolean by contract, so the action stashes the
  // created record for the success path to hand to onCreated.
  const lastCreated = React.useRef<CaseRecord | null>(null);
  const create = useMutation(async (input: {
    title: string;
    summary: string;
    severity: CaseRecord["severity"];
    clusterFsid?: string;
  }) => {
    lastCreated.current = await createCase(input, token);
  });

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (title.trim() === "" || summary.trim() === "") {
      return;
    }
    lastCreated.current = null;
    const ok = await create.run({
      title: title.trim(),
      summary: summary.trim(),
      severity,
      clusterFsid: clusterFsid.trim() === "" ? undefined : clusterFsid.trim(),
    });
    const created = lastCreated.current;
    if (ok && created) {
      setTitle("");
      setSummary("");
      setSeverity("medium");
      setClusterFsid(defaultClusterFsid ?? "");
      onCreated(created);
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
          <Button
            type="submit"
            disabled={create.busy || title.trim() === "" || summary.trim() === ""}
          >
            {create.busy ? "Creating…" : "Create case"}
          </Button>
          {create.error ? <p className="atlas-form-error">{create.error}</p> : null}
        </div>
      </form>
    </section>
  );
}
