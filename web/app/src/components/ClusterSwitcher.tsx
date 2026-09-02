import React from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { Dropdown, HeaderGlobalBar } from "@carbon/react";
import { listClusters } from "../api";
import { clusterFsidFromPath } from "../clusters";
import { useResource } from "../resources";

// The cluster switcher (issue #42): one dropdown in the app shell that
// selects the cluster every section renders. Selection is the URL —
// picking a cluster navigates to its overview, and the current cluster
// route drives the displayed value — so no page can show one cluster's
// data while another is selected.
export function ClusterSwitcher() {
  const location = useLocation();
  const navigate = useNavigate();
  const selectedFSID = clusterFsidFromPath(location.pathname);
  // Refetch on navigation so registering or deregistering a cluster
  // from any page keeps the switcher's list honest. The list is one
  // API page (100 = the max); the searchable index remains the surface
  // for larger fleets.
  const clusters = useResource(
    (signal) => listClusters({ limit: 100 }, signal),
    [location.pathname],
  );

  const items = (clusters.data?.clusters ?? []).flatMap((cluster) =>
    cluster.fsid ? [{ id: cluster.fsid, text: cluster.name }] : [],
  );
  const selectedItem = items.find((item) => item.id === selectedFSID) ?? null;

  return (
    <HeaderGlobalBar>
      <Dropdown
        id="atlas-cluster-switcher"
        label="Select cluster"
        titleText="Cluster"
        hideLabel
        invalidText="Clusters failed to load"
        items={items}
        selectedItem={selectedItem}
        itemToString={(item: { id: string; text: string } | null) =>
          item ? item.text : ""
        }
        onChange={(data: { selectedItem: { id: string; text: string } | null }) => {
          if (data.selectedItem && data.selectedItem.id !== selectedFSID) {
            navigate(`/clusters/${data.selectedItem.id}`);
          }
        }}
      />
    </HeaderGlobalBar>
  );
}
