import type { ReactNode } from "react";
import { InlineNotification, Tag, Tile } from "@carbon/react";
import { tagClassNameFor, tagTypeFor, type Tone } from "../tones";

// One Tag renders every tone. Carbon's Tag has no warning type, so warn
// tones carry the token-backed atlas-tag-warn class instead (ADR-0028
// deviation: no yellow Tag exists; the class uses Carbon support tokens).
export function StatusTag({
  label,
  tone,
  title,
}: {
  label: ReactNode;
  tone: Tone;
  title?: string;
}) {
  return (
    <Tag type={tagTypeFor(tone)} className={tagClassNameFor(tone)} title={title}>
      {label}
    </Tag>
  );
}

export function MetricTile({
  label,
  value,
  detail,
}: {
  label: string;
  value: ReactNode;
  detail?: ReactNode;
}) {
  return (
    <Tile className="atlas-metric-tile">
      <p className="atlas-metric-label">{label}</p>
      <p className="atlas-metric-value">{value}</p>
      {detail ? <p className="atlas-metric-detail">{detail}</p> : null}
    </Tile>
  );
}

export function ErrorState({ message }: { message: string }) {
  return (
    <InlineNotification
      kind="error"
      lowContrast
      title="Request failed"
      subtitle={message}
      role="alert"
    />
  );
}

export function EmptyState({ label }: { label: string }) {
  return <p className="atlas-empty">{label}</p>;
}

export function PageIntro({ children }: { children: ReactNode }) {
  return <p className="atlas-page-intro">{children}</p>;
}
