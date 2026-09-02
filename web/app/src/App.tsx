import React from "react";
import { Link, Navigate, Route, Routes } from "react-router-dom";
import {
  Content,
  Header,
  HeaderMenuItem,
  HeaderName,
  HeaderNavigation,
  Theme,
} from "@carbon/react";
import { OperatorProvider } from "./operator";
import { ClusterSwitcher } from "./components/ClusterSwitcher";
import { ClusterIndexPage } from "./pages/ClusterIndexPage";
import { ClusterDetailPage } from "./pages/ClusterDetailPage";
import { ClusterCasesPage } from "./pages/ClusterCasesPage";
import { ClusterSyncRunsPage } from "./pages/ClusterSyncRunsPage";
import { CasesPage } from "./pages/CasesPage";
import { SyncRunsPage } from "./pages/SyncRunsPage";

// The app shell is Carbon's UI shell (ADR-0028): one header, one content
// grid, Carbon components as the default vocabulary underneath. The
// cluster switcher selects the cluster every scoped section renders;
// the header nav stays fleet-wide (issue #42).
export function App() {
  return (
    <Theme theme="white">
      <Header aria-label="Atlas">
        <HeaderName as={Link} to="/" prefix="Atlas">
          Ceph operations
        </HeaderName>
        <HeaderNavigation aria-label="Atlas">
          <HeaderMenuItem as={Link} to="/">
            Clusters
          </HeaderMenuItem>
          <HeaderMenuItem as={Link} to="/cases">
            Cases
          </HeaderMenuItem>
          <HeaderMenuItem as={Link} to="/sync-runs">
            Sync runs
          </HeaderMenuItem>
        </HeaderNavigation>
        <ClusterSwitcher />
      </Header>
      <OperatorProvider>
        <Content className="atlas-content">
          <Routes>
            <Route path="/" element={<ClusterIndexPage />} />
            <Route path="/clusters/:fsid" element={<ClusterDetailPage />} />
            <Route path="/clusters/:fsid/cases" element={<ClusterCasesPage />} />
            <Route
              path="/clusters/:fsid/sync-runs"
              element={<ClusterSyncRunsPage />}
            />
            <Route path="/cases" element={<CasesPage />} />
            <Route path="/sync-runs" element={<SyncRunsPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Content>
      </OperatorProvider>
    </Theme>
  );
}
