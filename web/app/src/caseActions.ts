import type { CaseRecord } from "./api";

export type CaseActionsForStatus = {
  canTriage: boolean;
  canClose: boolean;
  isClosed: boolean;
  canAssign: boolean;
  canAttachWorkflow: boolean;
};

export function availableCaseActions(status: CaseRecord["status"]): CaseActionsForStatus {
  switch (status) {
    case "detected":
      return {
        canTriage: true,
        canClose: true,
        isClosed: false,
        canAssign: true,
        canAttachWorkflow: true,
      };
    case "triaged":
      return {
        canTriage: false,
        canClose: true,
        isClosed: false,
        canAssign: true,
        canAttachWorkflow: true,
      };
    case "closed":
      return {
        canTriage: false,
        canClose: false,
        isClosed: true,
        canAssign: false,
        canAttachWorkflow: false,
      };
  }
}
