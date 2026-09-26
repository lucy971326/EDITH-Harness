import { useCallback, useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  AlertDialog, AlertDialogContent, AlertDialogHeader, AlertDialogTitle,
  AlertDialogDescription, AlertDialogFooter, AlertDialogCancel, AlertDialogAction,
} from "@/components/ui/alert-dialog";
import { ArrowLeft } from "../icons";
import type { NavigationGuard } from "../workspace/use-page-navigation";
import { createSettingsSections, type SettingsContentProps, type SettingsSection } from "./sections";
import type { SettingsDraftState } from "./types";

function SectionContent({ section, onStateChange }: {
  section: SettingsSection;
  onStateChange: (id: string, state: SettingsDraftState) => void;
}) {
  const reportState = useCallback((state: SettingsDraftState) => {
    onStateChange(section.id, state);
  }, [onStateChange, section.id]);
  return <TabsContent value={section.id} forceMount>{section.render(reportState)}</TabsContent>;
}

export function SettingsPage({ onBack, setNavigationGuard, ...contentProps }: SettingsContentProps & {
  onBack: () => void;
  setNavigationGuard: (guard: NavigationGuard | null) => void;
}) {
  const sections = createSettingsSections(contentProps);
  const [states, setStates] = useState<Record<string, SettingsDraftState>>({});
  const [pendingLeave, setPendingLeave] = useState<(() => void) | null>(null);
  const dirty = Object.values(states).some((state) => state.dirty);
  const saving = Object.values(states).some((state) => state.saving);
  const reportState = useCallback((id: string, state: SettingsDraftState) => {
    setStates((current) => {
      if (current[id]?.dirty === state.dirty && current[id]?.saving === state.saving) return current;
      return { ...current, [id]: state };
    });
  }, []);

  useEffect(() => {
    setNavigationGuard((proceed) => {
      if (saving) return;
      if (dirty) setPendingLeave(() => proceed);
      else proceed();
    });
    return () => setNavigationGuard(null);
  }, [dirty, saving, setNavigationGuard]);

  return (
    <section className="settings-page">
      <Tabs defaultValue={sections[0].id} orientation="vertical" className="settings-layout">
        <div className="settings-rail">
          <Button variant="ghost" className="settings-back" disabled={saving} onClick={onBack}>
            <ArrowLeft />返回工作台
          </Button>
          <h1>设置</h1>
          <TabsList className="settings-nav" aria-label="设置分类">
            {sections.map((section) => <TabsTrigger key={section.id} value={section.id}>
              <section.icon />{section.label}
            </TabsTrigger>)}
          </TabsList>
          <span className="settings-rail-caption">Harness · 工作台偏好</span>
        </div>
        <div className="settings-content">
          {sections.map((section) => <SectionContent key={section.id} section={section} onStateChange={reportState} />)}
        </div>
      </Tabs>
      <AlertDialog open={!!pendingLeave} onOpenChange={(open) => { if (!open) setPendingLeave(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>放弃更改并离开设置？</AlertDialogTitle>
            <AlertDialogDescription>还有未保存的修改。你可以继续编辑，或放弃这些修改。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>继续编辑</AlertDialogCancel>
            <AlertDialogAction disabled={saving} onClick={() => pendingLeave?.()}>放弃并离开</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}
