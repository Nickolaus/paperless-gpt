import React from "react";
import { Document } from "../DocumentProcessor";
import DocumentCard from "./DocumentCard";
import ArrowPathIcon from "@heroicons/react/24/outline/ArrowPathIcon";

export interface DocumentsToProcessProps {
  documents: Document[];
  selectedDocuments?: number[];
  onSelectDocument?: (documentId: number) => void;
  onSelectAll?: () => void;
  onSelectNone?: () => void;
  onReload?: () => void;
  disabled?: boolean;
  title?: string;
  filterTag?: string | null;
  gridCols?: string;
  children?: React.ReactNode;
}

const DocumentsToProcess: React.FC<DocumentsToProcessProps> = ({
  documents,
  selectedDocuments = [],
  onSelectDocument,
  onSelectAll,
  onSelectNone,
  onReload,
  disabled,
  title = "Documents",
  filterTag,
  gridCols = "grid-cols-1 md:grid-cols-2",
  children,
}) => {
  const [query, setQuery] = React.useState("");
  const normalizedQuery = query.trim().toLowerCase();
  const visibleDocuments = normalizedQuery
    ? documents.filter((document) =>
        [
          document.title,
          document.content,
          document.correspondent,
          document.document_type_name,
          document.created_date,
          document.original_file_name,
          ...(document.tags || []),
        ]
          .filter(Boolean)
          .join(" ")
          .toLowerCase()
          .includes(normalizedQuery)
      )
    : documents;

  return (
    <section>
      <div className="mb-4 rounded-md border border-gray-200 bg-white p-4 shadow-sm dark:border-gray-700 dark:bg-gray-800">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h2 className="text-xl font-semibold text-gray-900 dark:text-gray-100">{title}</h2>
            <p className="mt-1 text-sm text-gray-600 dark:text-gray-300">
              {selectedDocuments.length} of {documents.length} selected
              {filterTag ? (
                <>
                  {" "}from tag{" "}
                  <span className="rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-800 dark:bg-blue-900 dark:text-blue-200">
                    {filterTag}
                  </span>
                </>
              ) : null}
            </p>
          </div>

          <div className="flex flex-col gap-2 sm:flex-row">
            <label className="sr-only" htmlFor="document-search">
              Search documents
            </label>
            <input
              id="document-search"
              type="search"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search documents"
              className="rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-900 dark:text-gray-100"
            />
            <button
              type="button"
              onClick={onSelectAll}
              disabled={disabled || !onSelectAll || documents.length === 0}
              className="rounded-md border border-gray-300 px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-700"
            >
              Select all
            </button>
            <button
              type="button"
              onClick={onSelectNone}
              disabled={disabled || !onSelectNone || selectedDocuments.length === 0}
              className="rounded-md border border-gray-300 px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-700"
            >
              Select none
            </button>
            {onReload && (
              <button
                type="button"
                onClick={onReload}
                disabled={disabled}
                className="inline-flex items-center justify-center rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-800 dark:hover:bg-blue-900"
                aria-label="Reload documents"
              >
                <ArrowPathIcon className="h-5 w-5" />
              </button>
            )}
          </div>
        </div>
      </div>

      {children}

      <div className={`grid ${gridCols} gap-4`}>
        {visibleDocuments.map((doc) => (
          <DocumentCard
            key={doc.id}
            document={doc}
            isSelected={selectedDocuments.includes(doc.id)}
            onSelect={() => onSelectDocument && onSelectDocument(doc.id)}
            disabled={disabled}
          />
        ))}
      </div>
    </section>
  );
};

export default DocumentsToProcess;
