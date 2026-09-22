// 对应 kernel/permissions 与 kernel/approvals；申请数据不是已授予权限。
import type { PermissionMode } from "./harness.ts";

export interface PermissionModeChoice {
  id: PermissionMode;
  label: string;
  available: boolean;
}

export interface Policy {
  unrestricted: boolean;
  writeRoots: string[];
  network: boolean;
}
export interface ExtraPermissions {
  writeRoots?: string[];
  network?: boolean;
}
export interface PendingApproval {
  id: string;
  sessionID: string;
  runID: string;
  toolCallID: string;
  request: {
    toolName: string;
    arguments: Record<string, unknown>;
    workdir: string;
    reason: string;
    current: Policy;
    requested: ExtraPermissions;
  };
}
export interface ApprovalNotification {
  subscriptionID: string;
  event: PendingApproval[];
}
