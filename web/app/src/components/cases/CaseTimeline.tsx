import React from "react";
import type { TimelineEvent } from "../../api";
import { formatDate } from "../../format";
import { labelForTimelineEventType, timelinePayloadLabels } from "../../timeline";
import { EmptyState } from "../ui";

export function CaseTimeline({
  error,
  loading,
  timeline,
}: {
  error: string | null;
  loading: boolean;
  timeline: TimelineEvent[];
}) {
  return (
    <section className="atlas-case-subsection" aria-label="Case Timeline Events">
      <h3>
        Timeline Events{" "}
        {!loading && !error ? <span className="atlas-count">{timeline.length}</span> : null}
      </h3>
      {loading ? <EmptyState label="Loading Timeline Events." /> : null}
      {error ? <EmptyState label={`Timeline Events unavailable: ${error}`} /> : null}
      {!loading && !error && timeline.length === 0 ? (
        <EmptyState label="No Timeline Events recorded." />
      ) : null}
      {!loading && !error && timeline.length > 0 ? (
        <ol className="atlas-timeline">
          {timeline.map((event) => (
            <TimelineEventRow key={event.id} event={event} />
          ))}
        </ol>
      ) : null}
    </section>
  );
}

function TimelineEventRow({ event }: { event: TimelineEvent }) {
  const payloadLabels = timelinePayloadLabels(event);

  return (
    <li className="atlas-timeline-event">
      <div className="atlas-timeline-marker" aria-hidden="true" />
      <div className="atlas-timeline-body">
        <div className="atlas-timeline-heading">
          <strong>{labelForTimelineEventType(event.type)}</strong>
          <time dateTime={event.occurredAt}>{formatDate(event.occurredAt)}</time>
        </div>
        <p>{event.message}</p>
        <div className="atlas-timeline-meta">
          <span>{event.actor.displayName}</span>
          <span>{event.actor.type.replace("_", " ")}</span>
          {payloadLabels.map((label) => (
            <span key={label}>{label}</span>
          ))}
        </div>
      </div>
    </li>
  );
}
