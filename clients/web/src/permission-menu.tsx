import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { Check, ChevronDown, Shield } from "./icons";
import type { PermissionMode } from "../../contracts/harness.ts";
import type { PermissionModeChoice } from "../../contracts/approvals.ts";

export function PermissionMenu({ modes, value, disabled, onChange }: {
  modes: PermissionModeChoice[];
  value: PermissionMode;
  disabled: boolean;
  onChange: (mode: PermissionMode) => void;
}) {
  const label = modes.find((mode) => mode.id === value)?.label ?? "权限模式";
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" className="permission-trigger" disabled={disabled || !modes.length}
          aria-label={`权限模式：${label}`} title={label}>
          <Shield />
          <span className="permission-label">{label}</span>
          <ChevronDown />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" side="top">
        {modes.map((mode) => (
          <DropdownMenuItem key={mode.id} disabled={disabled || !mode.available}
            onSelect={() => onChange(mode.id)}>
            <span>{mode.label}</span>
            {!mode.available && <span className="muted">暂不可用</span>}
            {mode.id === value && <Check className="ml-auto" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
