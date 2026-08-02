import {
  CheckCircleIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  DocumentIcon,
  MagnifyingGlassPlusIcon,
  XMarkIcon,
} from "@heroicons/react/24/outline";
import React, { useRef, useState } from "react";
import {
  DocumentSuggestion,
  DocumentTypeOption,
  TagOption,
} from "../../DocumentProcessor";
import Button from "../ui/Button";
import ImageZoomModal from "../ui/ImageZoomModal";
import Modal from "../ui/Modal";
import {
  Decision,
  FieldKey,
  SuggestionEditHandlers,
  countChanges,
} from "./fields";
import SuggestionFields from "./SuggestionFields";

interface FocusReviewProps {
  items: DocumentSuggestion[];
  index: number;
  decisions: Record<number, Decision>;
  excludedMap: Record<number, Set<FieldKey>>;
  availableTags: TagOption[];
  tagSelectionMode: "all" | "applicable";
  tagDerivedParents: boolean;
  createNewTagsEnabled: boolean;
  availableDocumentTypes: DocumentTypeOption[];
  createNewDocumentTypesEnabled: boolean;
  handlers: SuggestionEditHandlers;
  onToggleField: (docId: number, key: FieldKey) => void;
  onApply: (docId: number) => void;
  onSkip: (docId: number) => void;
  onNavigate: (index: number) => void;
  onClose: () => void;
  applyingIds: Set<number>;
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return (
    target.tagName === "INPUT" ||
    target.tagName === "TEXTAREA" ||
    target.isContentEditable
  );
}

/**
 * One document at a time: the scan and its OCR text on the left ("show the
 * work"), the field diffs on the right, and a keyboard-first flow (j/k/a/s).
 */
const FocusReview: React.FC<FocusReviewProps> = ({
  items,
  index,
  decisions,
  excludedMap,
  availableTags,
  tagSelectionMode,
  tagDerivedParents,
  createNewTagsEnabled,
  availableDocumentTypes,
  createNewDocumentTypesEnabled,
  handlers,
  onToggleField,
  onApply,
  onSkip,
  onNavigate,
  onClose,
  applyingIds,
}) => {
  const item = items[index];
  // Track preview failures per document so navigating resets the fallback.
  const [previewFailedFor, setPreviewFailedFor] = useState<Set<number>>(
    () => new Set()
  );
  const [zoomOpen, setZoomOpen] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);

  if (!item) return null;

  const previewFailed = previewFailedFor.has(item.id);
  const previewSrc = `./api/documents/${item.id}/pages/0/image`;
  const previewAlt = `Scan preview of "${item.original_document.title}"`;

  const decision = decisions[item.id] || "pending";
  const excluded = excludedMap[item.id] || new Set<FieldKey>();
  const changeCount = countChanges(item, excluded);
  const applying = applyingIds.has(item.id);

  const goPrev = () => {
    setZoomOpen(false);
    onNavigate(index > 0 ? index - 1 : items.length - 1);
  };
  const goNext = () => {
    setZoomOpen(false);
    onNavigate(index < items.length - 1 ? index + 1 : 0);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (isEditableTarget(e.target) || zoomOpen) return;
    switch (e.key) {
      case "j":
      case "ArrowRight":
        e.preventDefault();
        goNext();
        break;
      case "k":
      case "ArrowLeft":
        e.preventDefault();
        goPrev();
        break;
      case "a":
        e.preventDefault();
        if (decision === "pending" && !applying) {
          onApply(item.id);
          goNext();
        }
        break;
      case "s":
        e.preventDefault();
        if (decision === "pending" && !applying) {
          onSkip(item.id);
          goNext();
        }
        break;
      case "e": {
        e.preventDefault();
        const input = panelRef.current?.querySelector<HTMLInputElement>(
          "input[type='text']:not(:disabled)"
        );
        input?.focus();
        input?.select();
        break;
      }
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      size="focus"
      hideTitle
      title={`Focus view: ${item.original_document.title}`}
      initialFocus={panelRef}
    >
      {/* Focus starts on the panel itself (not the first input) so j/k/a/s work immediately. */}
      <div
        ref={panelRef}
        tabIndex={-1}
        className="flex min-h-0 flex-1 flex-col outline-none"
        onKeyDown={handleKeyDown}
      >
        <header className="flex shrink-0 items-center justify-between gap-4 border-b border-line px-5 py-3">
          <div className="flex min-w-0 items-center gap-3">
            <h3
              className="truncate text-sm font-medium"
              title={item.original_document.title}
            >
              {item.original_document.title}
            </h3>
            {decision === "applied" && (
              <span className="inline-flex shrink-0 items-center gap-1 rounded-full bg-pos-tint px-2 py-0.5 text-xs font-medium text-pos">
                <CheckCircleIcon className="h-3.5 w-3.5" aria-hidden="true" />
                Applied
              </span>
            )}
            {decision === "skipped" && (
              <span className="shrink-0 rounded-full bg-surface-2 px-2 py-0.5 text-xs font-medium text-muted">
                Skipped
              </span>
            )}
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <span className="mr-2 text-xs tabular-nums text-faint">
              Document {index + 1} of {items.length}
            </span>
            <button
              type="button"
              onClick={goPrev}
              aria-label="Previous document"
              className="rounded-md p-1.5 text-muted hover:bg-surface-2 hover:text-ink"
            >
              <ChevronLeftIcon className="h-4 w-4" aria-hidden="true" />
            </button>
            <button
              type="button"
              onClick={goNext}
              aria-label="Next document"
              className="rounded-md p-1.5 text-muted hover:bg-surface-2 hover:text-ink"
            >
              <ChevronRightIcon className="h-4 w-4" aria-hidden="true" />
            </button>
            <button
              type="button"
              onClick={onClose}
              aria-label="Close focus view"
              className="ml-1 rounded-md p-1.5 text-muted hover:bg-surface-2 hover:text-ink"
            >
              <XMarkIcon className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>
        </header>

        <div className="flex min-h-0 flex-1">
          <div className="hidden min-h-0 w-1/2 flex-col border-r border-line bg-surface-2 md:flex">
            <div className="min-h-0 flex-[3] overflow-hidden border-b border-line bg-surface-2 p-3">
              {previewFailed ? (
                <div className="flex h-full items-center justify-center rounded border border-line bg-surface">
                  <DocumentIcon
                    className="h-10 w-10 text-faint"
                    aria-hidden="true"
                  />
                </div>
              ) : (
                <button
                  type="button"
                  onClick={() => setZoomOpen(true)}
                  className="group relative block h-full w-full"
                  aria-label="Enlarge scan preview"
                >
                  <img
                    key={item.id}
                    src={previewSrc}
                    alt={previewAlt}
                    className="mx-auto h-full max-w-full rounded border border-line bg-surface object-contain shadow-card"
                    onError={() =>
                      setPreviewFailedFor((prev) => new Set(prev).add(item.id))
                    }
                  />
                  <span className="pointer-events-none absolute inset-0 flex items-center justify-center rounded bg-black/0 transition-colors duration-150 group-hover:bg-black/10">
                    <span className="flex items-center gap-1.5 rounded-full bg-black/60 px-2.5 py-1 text-xs font-medium text-white opacity-0 transition-opacity duration-150 group-hover:opacity-100">
                      <MagnifyingGlassPlusIcon className="h-3.5 w-3.5" aria-hidden="true" />
                      Enlarge
                    </span>
                  </span>
                </button>
              )}
            </div>
            <div className="min-h-0 flex-[2] overflow-y-auto p-4">
              <h4 className="text-xs font-medium text-muted">
                Extracted text (what the AI read)
              </h4>
              <p className="mt-2 whitespace-pre-wrap text-sm leading-relaxed text-ink">
                {item.original_document.content || "No text extracted."}
              </p>
            </div>
          </div>

          <div className="flex min-h-0 w-full flex-col md:w-1/2">
            <div className="min-h-0 flex-1 overflow-y-auto px-5 py-2">
              <SuggestionFields
                suggestion={item}
                availableTags={availableTags}
                tagSelectionMode={tagSelectionMode}
                tagDerivedParents={tagDerivedParents}
                createNewTagsEnabled={createNewTagsEnabled}
                availableDocumentTypes={availableDocumentTypes}
                createNewDocumentTypesEnabled={createNewDocumentTypesEnabled}
                excluded={excluded}
                onToggleField={onToggleField}
                handlers={handlers}
                disabled={decision !== "pending" || applying}
              />
            </div>
            <footer className="flex shrink-0 items-center justify-between gap-3 border-t border-line px-5 py-3">
              <p className="hidden text-xs text-faint sm:block" aria-hidden="true">
                <kbd className="rounded border border-line bg-surface-2 px-1">j</kbd>/
                <kbd className="rounded border border-line bg-surface-2 px-1">k</kbd>{" "}
                documents ·{" "}
                <kbd className="rounded border border-line bg-surface-2 px-1">a</kbd>{" "}
                apply ·{" "}
                <kbd className="rounded border border-line bg-surface-2 px-1">s</kbd>{" "}
                skip ·{" "}
                <kbd className="rounded border border-line bg-surface-2 px-1">esc</kbd>{" "}
                close
              </p>
              {decision === "pending" ? (
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={applying}
                    onClick={() => {
                      onSkip(item.id);
                      goNext();
                    }}
                  >
                    Skip
                  </Button>
                  <Button
                    size="sm"
                    variant="primary"
                    loading={applying}
                    onClick={() => {
                      onApply(item.id);
                      goNext();
                    }}
                  >
                    Apply
                    {changeCount > 0 && (
                      <span className="tabular-nums">({changeCount})</span>
                    )}
                  </Button>
                </div>
              ) : (
                <span className="text-xs text-faint">
                  Decided — use j/k to keep reviewing.
                </span>
              )}
            </footer>
          </div>
        </div>
      </div>
      <ImageZoomModal
        key={previewSrc}
        open={zoomOpen}
        onClose={() => setZoomOpen(false)}
        src={previewSrc}
        alt={previewAlt}
        title={item.original_document.title}
      />
    </Modal>
  );
};

export default FocusReview;
