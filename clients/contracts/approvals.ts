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
  reviewReason?: string;
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
  mcp?: {
    kind: "config" | "call";
    workspace: string;
    source?: string;
    digest?: string;
    servers?: { name: string; target: string }[];
    server?: string;
    tool?: string;
    arguments?: Record<string, unknown>;
  };
}

export interface ApprovalSettings {
  engine: "llm" | "jev";
  model: string;
  reasoningEffort: string;
}
export interface ApprovalSettingsView {
  settings: ApprovalSettings;
  jevConfigured: boolean;
  available: boolean;
}
export interface ApprovalNotification {
  subscriptionID: string;
  event: PendingApproval[];
}
