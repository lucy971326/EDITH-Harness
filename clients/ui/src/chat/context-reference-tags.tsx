import { useState } from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { ChevronRight, FileText, Folder, MessageSquareQuote, X } from "../icons";
import { referenceLabel, type ContextReference } from "./context-references";

export function ContextReferenceTag({
  reference,
  onRemove,
  assistantNumber,
}: {
  reference: ContextReference;
  onRemove?: () => void;
  assistantNumber?: number;
}) {
  const [expanded, setExpanded] = useState(false);
  const label = reference.kind === "assistant-selection"
    ? `引用 ${assistantNumber ?? 1}` : referenceLabel(reference);
  const Icon = reference.kind === "directory" ? Folder
    : reference.kind === "assistant-selection" ? MessageSquareQuote : FileText;
  return (
    <div className="context-reference" data-expanded={expanded}>
      <div className="context-reference-tag">
        <Icon />
        {reference.kind === "selection" ? (
          <button
            type="button"
            className="context-reference-label"
            title={label}
            aria-label={`预览代码 ${label}`}
            aria-expanded={expanded}
            onClick={() => setExpanded((value) => !value)}
          >
            <span>{label}</span>
            <ChevronRight className={expanded ? "rotate-90" : ""} />
          </button>
        ) : reference.kind === "assistant-selection" ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="context-reference-label context-reference-quote" tabIndex={0}>
                {label}
              </span>
            </TooltipTrigger>
            <TooltipContent className="context-reference-tooltip" side="top" sideOffset={6}>
              {reference.comment && (
                <>
                  <span>用户评论：</span>
                  <pre>{reference.comment}</pre>
                  <div className="context-reference-tooltip-separator" role="separator" />
                </>
              )}
              <span>所选文本：</span>
              <pre>{reference.content}</pre>
            </TooltipContent>
          </Tooltip>
        ) : <span className="context-reference-label" title={label}>{label}</span>}
        {onRemove && (
          <button
            type="button"
            className="context-reference-remove"
            aria-label={`移除引用 ${label}`}
            onClick={onRemove}
          >
            <X />
          </button>
        )}
      </div>
      {expanded && reference.kind === "selection" && (
        <pre className="context-reference-code" tabIndex={0}>{reference.content}</pre>
      )}
    </div>
  );
}
