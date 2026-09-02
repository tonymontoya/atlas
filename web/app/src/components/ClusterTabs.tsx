import React from "react";
import { useNavigate } from "react-router-dom";
import { Tab, TabList, Tabs } from "@carbon/react";

// The cluster section tabs (issue #42): Overview, Cases, and Sync runs
// scoped to one cluster. The active tab comes from the route, and
// selecting a tab navigates — the URL stays the single source of which
// cluster's data a section shows.
const SECTIONS = [
  { suffix: "", label: "Overview" },
  { suffix: "/cases", label: "Cases" },
  { suffix: "/sync-runs", label: "Sync runs" },
] as const;

export type ClusterSection = (typeof SECTIONS)[number]["label"];

export function ClusterTabs({ fsid, active }: { fsid: string; active: ClusterSection }) {
  const navigate = useNavigate();
  const selectedIndex = SECTIONS.findIndex((section) => section.label === active);

  return (
    <Tabs
      selectedIndex={selectedIndex < 0 ? 0 : selectedIndex}
      onChange={(data: { selectedIndex: number }) => {
        const section = SECTIONS[data.selectedIndex];
        if (section) {
          navigate(`/clusters/${fsid}${section.suffix}`);
        }
      }}
    >
      <TabList aria-label="Cluster sections">
        {SECTIONS.map((section) => (
          <Tab key={section.label}>{section.label}</Tab>
        ))}
      </TabList>
    </Tabs>
  );
}
