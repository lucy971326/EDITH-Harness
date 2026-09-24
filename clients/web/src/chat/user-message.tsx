import { decodeReferences } from "./context-references";
import { ContextReferenceTag } from "./context-reference-tags";
import { MessageMarkdown } from "./message-markdown";
import type { FileLocation } from "../editor/links";

// 只有用户消息会解释引用段；助手输出及工具结果保持原有呈现。
export function UserMessage({
  text,
  workspace,
  onOpenFile,
}: {
  text: string;
  workspace?: string | null;
  onOpenFile?: (location: FileLocation) => void;
}) {
  const decoded = decodeReferences(text);
  let assistantReferenceNumber = 0;
  const references = decoded.references.map((reference) => ({
    reference,
    assistantNumber: reference.kind === "assistant-selection"
      ? ++assistantReferenceNumber : undefined,
  }));
  return (
    <>
      {decoded.references.length > 0 && (
        <div className="context-references" aria-label="引用上下文">
          {references.map(({ reference, assistantNumber }, index) => (
            <ContextReferenceTag key={index} reference={reference} assistantNumber={assistantNumber} />
          ))}
        </div>
      )}
      {decoded.text.trim() && (
        <MessageMarkdown text={decoded.text} workspace={workspace} onOpenFile={onOpenFile} />
      )}
    </>
  );
}
