// Generated from Go contracts. Do not edit.
import type { HarnessSessionCreateParams } from './harness-session-create.params';
import type { HarnessSessionCreateResult } from './harness-session-create.result';
import type { HarnessSessionGetParams } from './harness-session-get.params';
import type { HarnessSessionGetResult } from './harness-session-get.result';
import type { HarnessSessionListParams } from './harness-session-list.params';
import type { HarnessSessionListResult } from './harness-session-list.result';

export interface Methods {
  'harness/session/create': { params: HarnessSessionCreateParams; result: HarnessSessionCreateResult };
  'harness/session/get': { params: HarnessSessionGetParams; result: HarnessSessionGetResult };
  'harness/session/list': { params: HarnessSessionListParams; result: HarnessSessionListResult };
}
