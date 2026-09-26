import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { Check, Shield } from "../icons";
import type { PermissionMode } from "../../../contracts/harness.ts";
import type { PermissionModeChoice } from "../../../contracts/approvals.ts";

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
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="permission-popover" align="start" side="top" collisionPadding={12} aria-label="权限模式">
        <div className="picker-heading">
          <h3>权限模式</h3>
          <p>选择 Agent 可执行操作的范围。</p>
        </div>
        {modes.map((mode) => (
          <DropdownMenuItem className="permission-option" key={mode.id} disabled={disabled || !mode.available}
            data-selected={mode.id === value}
            onSelect={() => onChange(mode.id)}>
            <span>{mode.label}{!mode.available && <small>暂不可用</small>}</span>
            {mode.id === value && <Check />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
