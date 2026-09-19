import { useState } from "react";
import { ChevronRight, FileText, Folder, X } from "./icons";
import { referenceLabel, type ContextReference } from "./context-references";

export function ContextReferenceTag({
  reference,
  onRemove,
}: {
  reference: ContextReference;
  onRemove?: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const label = referenceLabel(reference);
  const Icon = reference.kind === "directory" ? Folder : FileText;
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
