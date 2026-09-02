import React from "react";
import { Link } from "react-router-dom";
import { InlineNotification } from "@carbon/react";
import { PageIntro } from "./ui";
import { ClusterTabs, type ClusterSection } from "./ClusterTabs";

// The shared chrome of every cluster-scoped page: the way back to the
// fleet index, the page title and intro, and the section tabs.
export function ClusterPageHeader({
  fsid,
  active,
  title,
  intro,
}: {
  fsid: string;
  active: ClusterSection;
  title: React.ReactNode;
  intro: React.ReactNode;
}) {
  return (
    <>
      <p>
        <Link to="/" className="atlas-table-link">
          ← All clusters
        </Link>
      </p>
      <h1 className="atlas-page-title">{title}</h1>
      <PageIntro>{intro}</PageIntro>
      <ClusterTabs fsid={fsid} active={active} />
    </>
  );
}

export function ClusterNotFound({ fsid }: { fsid: string }) {
  return (
    <div>
      <InlineNotification
        kind="error"
        lowContrast
        title="Cluster not found"
        subtitle={`No registered cluster carries the FSID ${fsid}. It may have been deregistered, or the link is stale.`}
      />
      <Link className="atlas-table-link" to="/">
        ← Back to all clusters
      </Link>
    </div>
  );
}
