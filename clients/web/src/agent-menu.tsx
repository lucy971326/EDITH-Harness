import { Button } from "@/components/ui/button";
import { useState } from "react";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Bot, Check, ChevronDown } from "./icons";
import type { AgentView } from "../../contracts/appserver.ts";

export function AgentMenu({
  agents,
  value,
  disabled,
  onChange,
}: {
  agents: AgentView[] | null;
  value: string;
  disabled: boolean;
  onChange: (agentID: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const selected = agents?.find((agent) => agent.id === value);
  return (
    <Popover open={open && !disabled} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          className="agent-select"
          disabled={disabled}
          aria-label="选择 Agent"
        >
          <Bot />
          {selected?.name ?? (value || "选择 Agent")}
          <ChevronDown />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="agent-popover" align="start" side="top">
        <h3>Agent</h3>
        <p className="metadata">选择后立即保存，用于下一轮。</p>
        <div className="agent-options" role="group" aria-label="Agent">
          {agents?.map((agent) => (
            <button
              key={agent.id}
              aria-pressed={agent.id === value}
              onClick={() => {
                onChange(agent.id);
                setOpen(false);
              }}
            >
              <span>{agent.name}</span>
              {agent.id === value && <Check />}
            </button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}
