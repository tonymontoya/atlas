import React from "react";
import {
  Checkbox,
  CodeSnippet,
  InlineNotification,
  Modal,
  RadioButton,
  RadioButtonGroup,
  TextInput,
} from "@carbon/react";
import {
  registerCluster,
  type ClusterType,
  type CreateClusterRegistrationResponse,
} from "../../api";
import { agentInstallInstructions, credentialExpiresLabel } from "../../registrations";
import { useMutation } from "../../resources";

// RegisterClusterModal is the two-step registration flow (ADR-0025/0026):
// a form step (name, type), then the one-time Enrollment Credential with
// install instructions. The credential lives only in this component's
// state, captured from the registration response — closing or reopening
// the modal can never re-display it, because nothing else ever returns
// it. The modal mounts only while open, so every reopen starts from a
// clean form. Done is gated on the operator acknowledging the copy.
export function RegisterClusterModal({
  token,
  onClose,
  onRegistered,
}: {
  token: string;
  onClose: () => void;
  onRegistered: () => void;
}) {
  const [name, setName] = React.useState("");
  const [clusterType, setClusterType] = React.useState<ClusterType>("bare-metal");
  const [acknowledged, setAcknowledged] = React.useState(false);
  // Non-null means the credential step: the response is held in memory
  // only for as long as the modal stays open.
  const [result, setResult] = React.useState<CreateClusterRegistrationResponse | null>(
    null,
  );
  const lastResult = React.useRef<CreateClusterRegistrationResponse | null>(null);
  const register = useMutation(async (input: { name: string; clusterType: ClusterType }) => {
    lastResult.current = await registerCluster(input, token);
  });

  async function submit() {
    if (register.busy || name.trim() === "") {
      return;
    }
    lastResult.current = null;
    const ok = await register.run({ name: name.trim(), clusterType });
    const created = lastResult.current;
    if (ok && created) {
      setResult(created);
      // The row exists from here on; the index refreshes behind the
      // modal so the list is current whenever the operator closes it.
      onRegistered();
    }
  }

  if (result) {
    return (
      <Modal
        open
        modalHeading={`Cluster “${result.cluster.name}” registered`}
        modalLabel="Registration complete"
        primaryButtonText="Done"
        primaryButtonDisabled={!acknowledged}
        secondaryButtonText="Discard and close"
        onRequestSubmit={onClose}
        onRequestClose={onClose}
        preventCloseOnClickOutside
      >
        <InlineNotification
          kind="warning"
          lowContrast
          title="Save the Enrollment Credential now"
          subtitle="This is the only time it is shown. It works once, cannot be re-displayed, and the Agent cannot enroll without it."
        />
        <div className="atlas-credential">
          <CodeSnippet
            type="multi"
            wrapText
            feedback="Credential copied"
            ariaLabel="One-time enrollment credential"
            copyButtonDescription="Copy enrollment credential"
          >
            {result.enrollmentCredential.token}
          </CodeSnippet>
          <p className="atlas-subtle">
            {credentialExpiresLabel(result.enrollmentCredential.expiresAt)} · the
            credential is consumed by enrollment, expiry, or deregistration
          </p>
        </div>
        <h3 className="atlas-panel-heading">Install the Agent</h3>
        <CodeSnippet
          type="multi"
          wrapText
          hideCopyButton
          ariaLabel="Agent install instructions"
          className="atlas-instructions"
        >
          {agentInstallInstructions()}
        </CodeSnippet>
        <Checkbox
          id="credential-acknowledged"
          labelText="I have saved the Enrollment Credential. I understand it will not be shown again."
          checked={acknowledged}
          onChange={(_, { checked }) => setAcknowledged(checked)}
        />
      </Modal>
    );
  }

  return (
    <Modal
      open
      modalHeading="Register cluster"
      modalLabel="Registration"
      primaryButtonText={register.busy ? "Registering…" : "Register cluster"}
      primaryButtonDisabled={register.busy || name.trim() === ""}
      secondaryButtonText="Cancel"
      onRequestSubmit={submit}
      onRequestClose={onClose}
    >
      {register.error ? (
        <InlineNotification
          kind="error"
          lowContrast
          title="Registration failed"
          subtitle={register.error}
          role="alert"
        />
      ) : null}
      <TextInput
        id="register-cluster-name"
        labelText="Cluster name"
        placeholder="storage-a"
        value={name}
        onChange={(event) => setName(event.target.value)}
      />
      <RadioButtonGroup
        legendText="Cluster type"
        name="register-cluster-type"
        valueSelected={clusterType}
        onChange={(value) => setClusterType(String(value) as ClusterType)}
      >
        <RadioButton labelText="Bare-metal" value="bare-metal" />
        <RadioButton labelText="Rook (arrives in v0.3.0)" value="rook" disabled />
      </RadioButtonGroup>
    </Modal>
  );
}
