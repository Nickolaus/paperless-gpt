import React from "react";
import { Document } from "../DocumentProcessor";

interface DocumentCardProps {
  document: Document;
  isSelected?: boolean;
  onSelect?: (documentId: number) => void;
  disabled?: boolean;
}

const metadataItems = (document: Document) => [
  { label: "ID", value: document.id ? String(document.id) : "" },
  { label: "Correspondent", value: document.correspondent },
  { label: "Type", value: document.document_type_name },
  { label: "Created", value: document.created_date },
  { label: "File", value: document.original_file_name },
].filter((item) => item.value);

const DocumentCard: React.FC<DocumentCardProps> = ({ document, isSelected, onSelect, disabled }) => (
  <article
    className={`document-card rounded-md border bg-white p-4 shadow-sm transition dark:bg-gray-800 ${
      isSelected
        ? "border-blue-500 ring-2 ring-blue-500"
        : "border-gray-200 dark:border-gray-700"
    } ${onSelect && !disabled ? "cursor-pointer hover:border-blue-400" : ""} ${disabled ? "opacity-60" : ""}`}
    onClick={() => onSelect && !disabled && onSelect(document.id)}
  >
    {onSelect && (
      <div className="mb-3 flex items-start justify-between gap-3">
        <label
          className="flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-200"
          onClick={(e) => e.stopPropagation()}
        >
          <input
            type="checkbox"
            checked={Boolean(isSelected)}
            disabled={disabled}
            onChange={() => onSelect(document.id)}
            onClick={(e) => e.stopPropagation()}
            className="h-5 w-5 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
          />
          Select document
        </label>
      </div>
    )}

    <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100">{document.title || "Untitled document"}</h3>

    {metadataItems(document).length > 0 && (
      <dl className="mt-3 grid grid-cols-1 gap-x-4 gap-y-1 text-sm text-gray-600 dark:text-gray-300 sm:grid-cols-2">
        {metadataItems(document).map((item) => (
          <div key={item.label} className="min-w-0">
            <dt className="inline font-medium text-gray-700 dark:text-gray-200">{item.label}: </dt>
            <dd className="inline break-words">{item.value}</dd>
          </div>
        ))}
      </dl>
    )}

    <p className="mt-3 line-clamp-4 whitespace-pre-wrap text-sm text-gray-600 dark:text-gray-400">
      {document.content?.trim()
        ? document.content
        : "No extracted document text is available yet."}
    </p>

    <div className="mt-4 flex flex-wrap gap-2">
      {document.tags.map((tag) => (
        <span
          key={tag}
          className="rounded-full bg-blue-100 px-2.5 py-0.5 text-xs font-medium text-blue-800 dark:bg-blue-900 dark:text-blue-200"
        >
          {tag}
        </span>
      ))}
    </div>
  </article>
);

export default DocumentCard;
