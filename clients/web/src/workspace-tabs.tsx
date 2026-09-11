import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { FileText, Globe, Terminal, Plus, X } from "./icons";

const kinds = [
  { name: "文件", icon: FileText },
  { name: "浏览器", icon: Globe },
  { name: "终端", icon: Terminal },
] as const;
type WorkspaceTab = { id: string; kind: number; number: number };

// 独立工作区只管理标签，不绑定中间选中的会话。
export function WorkspaceTabs({ onHide }: { onHide: () => void }) {
  const [tabs, setTabs] = useState<WorkspaceTab[]>([]);
  const [active, setActive] = useState("");
  const [nextNumber, setNextNumber] = useState(1);

  function open(kind: number) {
    const tab = { id: crypto.randomUUID(), kind, number: nextNumber };
    setTabs([...tabs, tab]);
    setNextNumber(nextNumber + 1);
    setActive(tab.id);
  }
  function close(id: string) {
    const index = tabs.findIndex((tab) => tab.id === id);
    const remaining = tabs.filter((tab) => tab.id !== id);
    setTabs(remaining);
    if (id === active)
      setActive(remaining[Math.min(index, remaining.length - 1)]?.id ?? "");
  }

  return (
    <Tabs value={active} onValueChange={setActive} className="workspace-tabs">
      <header>
        {tabs.length > 0 && (
          <TabsList className="workspace-tab-list" aria-label="工作区标签页">
            {tabs.map((tab) => {
              const kind = kinds[tab.kind];
              return (
                <div
                  className="workspace-tab"
                  data-active={tab.id === active}
                  key={tab.id}
                >
                  <TabsTrigger value={tab.id}>
                    <kind.icon />
                    {kind.name} {tab.number}
                  </TabsTrigger>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={`关闭${kind.name} ${tab.number}`}
                    onClick={() => close(tab.id)}
                  >
                    <X />
                  </Button>
                </div>
              );
            })}
          </TabsList>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" aria-label="新建工作区标签">
              <Plus />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            {kinds.map((kind, index) => (
              <DropdownMenuItem key={kind.name} onSelect={() => open(index)}>
                <kind.icon />
                {kind.name}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
        <Button
          variant="ghost"
          size="icon"
          className="workspace-hide"
          aria-label="关闭辅助工作区"
          onClick={onHide}
        >
          <X />
        </Button>
      </header>
      {tabs.length === 0 ? (
        <div className="workspace-chooser" aria-label="选择新标签页类型">
          {kinds.map((kind, index) => (
            <Button key={kind.name} variant="ghost" onClick={() => open(index)}>
              <kind.icon />
              <span>{kind.name}</span>
              <Plus />
            </Button>
          ))}
        </div>
      ) : (
        tabs.map((tab) => {
          const kind = kinds[tab.kind];
          return (
            <TabsContent
              key={tab.id}
              value={tab.id}
              className="workspace-tab-content"
            >
              <kind.icon />
              <p>
                {kind.name} {tab.number}
              </p>
              <span>{kind.name}能力尚未接入</span>
            </TabsContent>
          );
        })
      )}
    </Tabs>
  );
}
