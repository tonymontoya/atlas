import type {
  ClusterHealth,
  Daemon,
  OSD,
  Pool,
  StorageDevice,
} from "../api";
import { poolRedundancyLabel, storageDeviceOSDLabel } from "../inventory";
import {
  toneForDaemonStatus,
  toneForDeviceHealth,
  toneForHealth,
} from "../tones";
import { StatusTag } from "./ui";
import { AtlasTable } from "./tables";

export function HealthChecksTable({ health }: { health: ClusterHealth }) {
  if (health.checks.length === 0) {
    return <p className="atlas-empty">No active health checks.</p>;
  }
  return (
    <AtlasTable
      columns={[
        { key: "name", header: "Check", render: (check) => check.name },
        {
          key: "severity",
          header: "Severity",
          render: (check) => (
            <StatusTag
              label={check.severity.replace("HEALTH_", "")}
              tone={toneForHealth(check.severity)}
            />
          ),
        },
        { key: "summary", header: "Summary", render: (check) => check.summary },
      ]}
      rows={health.checks}
      rowKey={(check) => `${check.name}-${check.summary}`}
      emptyLabel="No active health checks."
    />
  );
}

export function OSDTable({ osds }: { osds: OSD[] }) {
  return (
    <AtlasTable
      columns={[
        { key: "id", header: "ID", render: (osd) => osd.id },
        { key: "host", header: "Host", render: (osd) => osd.host },
        {
          key: "up",
          header: "Up",
          render: (osd) => (
            <StatusTag label={osd.up ? "yes" : "no"} tone={osd.up ? "ok" : "err"} />
          ),
        },
        {
          key: "in",
          header: "In",
          render: (osd) => (
            <StatusTag label={osd.in ? "yes" : "no"} tone={osd.in ? "ok" : "warn"} />
          ),
        },
        { key: "device", header: "Device", render: (osd) => osd.device ?? "unreported" },
      ]}
      rows={osds}
      rowKey={(osd) => String(osd.id)}
      emptyLabel="No OSDs returned."
    />
  );
}

export function StorageDeviceTable({ devices }: { devices: StorageDevice[] }) {
  return (
    <AtlasTable
      columns={[
        { key: "serial", header: "Serial", render: (device) => device.serial },
        { key: "host", header: "Host", render: (device) => device.host },
        { key: "type", header: "Type", render: (device) => device.type ?? "unknown" },
        { key: "path", header: "Path", render: (device) => device.path ?? "unreported" },
        {
          key: "health",
          header: "Health",
          render: (device) => (
            <StatusTag
              label={device.health ?? "unknown"}
              tone={toneForDeviceHealth(device.health)}
            />
          ),
        },
        {
          key: "backing",
          header: "Backing",
          render: (device) => storageDeviceOSDLabel(device),
        },
      ]}
      rows={devices}
      rowKey={(device) => `${device.host}-${device.serial}`}
      emptyLabel="No Storage Devices returned."
    />
  );
}

export function DaemonTable({ daemons }: { daemons: Daemon[] }) {
  return (
    <AtlasTable
      columns={[
        { key: "name", header: "Name", render: (daemon) => daemon.name },
        { key: "type", header: "Type", render: (daemon) => daemon.type },
        { key: "host", header: "Host", render: (daemon) => daemon.host },
        {
          key: "status",
          header: "Status",
          render: (daemon) => (
            <StatusTag
              label={daemon.status}
              tone={toneForDaemonStatus(daemon.status)}
            />
          ),
        },
        { key: "version", header: "Version", render: (daemon) => daemon.version ?? "unreported" },
      ]}
      rows={daemons}
      rowKey={(daemon) => `${daemon.type}-${daemon.name}`}
      emptyLabel="No Ceph Daemons returned."
    />
  );
}

export function PoolTable({ pools }: { pools: Pool[] }) {
  return (
    <AtlasTable
      columns={[
        { key: "id", header: "ID", render: (pool) => pool.id },
        { key: "name", header: "Name", render: (pool) => pool.name },
        { key: "type", header: "Type", render: (pool) => pool.type },
        { key: "redundancy", header: "Redundancy", render: (pool) => poolRedundancyLabel(pool) },
      ]}
      rows={pools}
      rowKey={(pool) => String(pool.id)}
      emptyLabel="No Pools returned."
    />
  );
}
