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
import { ClusterIndexPage } from "./pages/ClusterIndexPage";
import { ClusterDetailPage } from "./pages/ClusterDetailPage";
import { CasesPage } from "./pages/CasesPage";
import { SyncRunsPage } from "./pages/SyncRunsPage";

// The app shell is Carbon's UI shell (ADR-0028): one header, one content
// grid, Carbon components as the default vocabulary underneath.
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
      </Header>
      <OperatorProvider>
        <Content className="atlas-content">
          <Routes>
            <Route path="/" element={<ClusterIndexPage />} />
            <Route path="/clusters/:fsid" element={<ClusterDetailPage />} />
            <Route path="/cases" element={<CasesPage />} />
            <Route path="/sync-runs" element={<SyncRunsPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Content>
      </OperatorProvider>
    </Theme>
  );
}
