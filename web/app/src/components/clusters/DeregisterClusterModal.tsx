import React from "react";
import { InlineNotification, Modal } from "@carbon/react";
import { deregisterCluster } from "../../api";
import { useMutation } from "../../resources";

// DeregisterClusterModal confirms retiring a registration. The danger
// framing is about the Agent loop, not the history: deregistration
// consumes any live Enrollment Credential immediately, while inventory,
// Cases, and Timeline Events stay untouched. The modal mounts only
// while a cluster is selected, so each confirmation starts clean.
export function DeregisterClusterModal({
  cluster,
  token,
  onClose,
  onDeregistered,
}: {
  cluster: { id: number; name: string };
  token: string;
  onClose: () => void;
  onDeregistered: () => void;
}) {
  const deregister = useMutation((id: number) => deregisterCluster(id, token));

  async function confirm() {
    if (deregister.busy) {
      return;
    }
    const ok = await deregister.run(cluster.id);
    if (ok) {
      onDeregistered();
      onClose();
    }
  }

  return (
    <Modal
      open
      modalHeading={`Deregister “${cluster.name}”?`}
      modalLabel="Deregistration"
      primaryButtonText={deregister.busy ? "Deregistering…" : "Deregister"}
      primaryButtonDisabled={deregister.busy}
      secondaryButtonText="Cancel"
      danger
      onRequestSubmit={confirm}
      onRequestClose={() => {
        if (!deregister.busy) {
          onClose();
        }
      }}
    >
      {deregister.error ? (
        <InlineNotification
          kind="error"
          lowContrast
          title="Deregistration failed"
          subtitle={deregister.error}
          role="alert"
        />
      ) : null}
      <p>
        Retires the registration. Any live Enrollment Credential is consumed
        immediately and the cluster stops listing.
      </p>
      <p className="atlas-subtle">
        Historical inventory, Cases, and Timeline Events are preserved.
      </p>
    </Modal>
  );
}
