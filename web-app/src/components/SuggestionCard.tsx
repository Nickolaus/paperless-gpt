import React from "react";
import { ReactTags } from "react-tag-autocomplete";
import { DocumentSuggestion, DocumentTypeOption, TagOption } from "../DocumentProcessor";

interface SuggestionCardProps {
  suggestion: DocumentSuggestion;
  availableTags: TagOption[];
  availableDocumentTypes: DocumentTypeOption[];
  createNewDocumentTypesEnabled: boolean;
  onTitleChange: (docId: number, title: string) => void;
  onTagAddition: (docId: number, tag: TagOption) => void;
  onTagDeletion: (docId: number, tag: string) => void;
  onTagRestore: (docId: number, tag: string) => void;
  onCorrespondentChange: (docId: number, correspondent: string) => void;
  onDocumentTypeChange: (docId: number, documentType: string) => void;
  onCreatedDateChange: (docId: number, createdDate: string) => void;
  onCustomFieldSuggestionToggle: (docId: number, fieldId: number) => void;
}

const SuggestionCard: React.FC<SuggestionCardProps> = ({
  suggestion,
  availableTags,
  availableDocumentTypes,
  createNewDocumentTypesEnabled,
  onTitleChange,
  onTagAddition,
  onTagDeletion,
  onTagRestore,
  onCorrespondentChange,
  onDocumentTypeChange,
  onCreatedDateChange,
  onCustomFieldSuggestionToggle,
}) => {
  const sortedAvailableTags = [...availableTags].sort((a, b) => a.name.localeCompare(b.name));
  const sortedAvailableDocumentTypes = [...availableDocumentTypes].sort((a, b) => a.name.localeCompare(b.name));
  const document = suggestion.original_document;
  const originalValue = (value?: string) => value?.trim() || "Empty";
  const tagEquals = (left: string, right: string) => left.localeCompare(right, undefined, { sensitivity: "accent" }) === 0;
  const includesTag = (tags: string[], tag: string) => tags.some((candidate) => tagEquals(candidate, tag));
  const availableTagNames = availableTags.map((tag) => tag.name);
  const selectedTags = suggestion.suggested_tags || [];
  const originalTags = document.tags || [];
  const keptTags = originalTags.filter((tag) => includesTag(selectedTags, tag));
  const removedTags = (suggestion.remove_tags || []).filter((tag) => includesTag(originalTags, tag) && !includesTag(selectedTags, tag));
  const addedTags = selectedTags.filter((tag) => !includesTag(originalTags, tag));
  const suggestedExistingTags = addedTags.filter((tag) => includesTag(availableTagNames, tag));
  const newTags = addedTags.filter((tag) => !includesTag(availableTagNames, tag));
  const suggestedDocumentType = suggestion.suggested_document_type?.trim() || "";
  const suggestedDocumentTypeExists = suggestedDocumentType
    ? sortedAvailableDocumentTypes.some((documentType) => tagEquals(documentType.name, suggestedDocumentType))
    : false;

  const renderTagList = (
    tags: string[],
    emptyText: string,
    chipClassName: string,
    action?: (tag: string) => { label: string; onClick: () => void; className: string }
  ) => {
    if (tags.length === 0) {
      return <p className="text-xs text-gray-500 dark:text-gray-400">{emptyText}</p>;
    }

    return (
      <div className="flex flex-wrap gap-2">
        {tags.map((tag) => {
          const tagAction = action?.(tag);
          return (
            <span key={tag} className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium ${chipClassName}`}>
              <span>{tag}</span>
              {tagAction && (
                <button
                  type="button"
                  onClick={tagAction.onClick}
                  className={`rounded-full px-1.5 py-0.5 text-[11px] font-semibold ${tagAction.className}`}
                  aria-label={`${tagAction.label} ${tag}`}
                >
                  {tagAction.label}
                </button>
              )}
            </span>
          );
        })}
      </div>
    );
  };

  return (
    <div className="bg-white dark:bg-gray-800 shadow-lg shadow-blue-500/50 rounded-md p-4 relative flex flex-col justify-between h-full">
      <div className="flex items-center group relative">
        <div className="relative">
          <h3 className="text-lg font-semibold text-gray-800 dark:text-gray-200">
            {document.title}
          </h3>
          <p className="text-sm text-gray-600 dark:text-gray-400 mt-2 truncate">
            {document.content.length > 40
              ? `${document.content.substring(0, 40)}...`
              : document.content}
          </p>
          <div className="mt-4">
            {document.tags.map((tag) => (
              <span
                key={tag}
                className="bg-blue-100 dark:bg-blue-900 text-blue-800 dark:text-blue-200 text-xs font-medium mr-2 px-2.5 py-0.5 rounded-full"
              >
                {tag}
              </span>
            ))}
          </div>
        </div>
        <div className="absolute inset-0 bg-black bg-opacity-50 dark:bg-opacity-70 opacity-0 group-hover:opacity-100 transition-opacity duration-300 flex items-center justify-center p-4 rounded-md">
          <div className="text-sm text-white p-2 bg-gray-800 dark:bg-gray-900 rounded-md w-full max-h-full overflow-y-auto">
            <p className="mt-2 whitespace-pre-wrap">{document.content}</p>
          </div>
        </div>
      </div>
      <div className="mt-4">
        {suggestion.field_errors && Object.keys(suggestion.field_errors).length > 0 && (
          <div className="mb-4 rounded border border-yellow-300 bg-yellow-50 p-3 text-sm text-yellow-800 dark:border-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-100">
            <div className="font-semibold">Some suggestions failed</div>
            <ul className="mt-2 list-disc pl-5">
              {Object.entries(suggestion.field_errors).map(([field, message]) => (
                <li key={field}>
                  <span className="font-medium">{field}:</span> {message}
                </li>
              ))}
            </ul>
          </div>
        )}
        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
          Suggested Title
        </label>
        <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
          Current: {originalValue(document.title)}
        </p>
        <input
          type="text"
          value={suggestion.suggested_title || ""}
          onChange={(e) => onTitleChange(suggestion.id, e.target.value)}
          className="w-full border border-gray-300 dark:border-gray-600 rounded px-2 py-1 mt-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-gray-200"
        />
        <div className="mt-4">
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
            Suggested Tags
          </label>
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            Existing tags are kept unless you remove them. Suggested tags are added on top.
          </p>
          <div className="mt-3 space-y-3 rounded-md border border-gray-200 p-3 dark:border-gray-700">
            <div>
              <h4 className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">Existing tags kept</h4>
              {renderTagList(
                keptTags,
                "No existing tags will be kept.",
                "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-100",
                (tag) => ({
                  label: "Remove",
                  onClick: () => onTagDeletion(suggestion.id, tag),
                  className: "bg-gray-200 text-gray-700 hover:bg-red-100 hover:text-red-700 dark:bg-gray-600 dark:text-gray-100 dark:hover:bg-red-900 dark:hover:text-red-100",
                })
              )}
            </div>
            <div>
              <h4 className="mb-1 text-xs font-semibold uppercase tracking-wide text-blue-700 dark:text-blue-300">Suggested existing tags</h4>
              {renderTagList(
                suggestedExistingTags,
                "No existing Paperless tags were added.",
                "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-100",
                (tag) => ({
                  label: "Remove",
                  onClick: () => onTagDeletion(suggestion.id, tag),
                  className: "bg-blue-200 text-blue-800 hover:bg-red-100 hover:text-red-700 dark:bg-blue-800 dark:text-blue-100 dark:hover:bg-red-900 dark:hover:text-red-100",
                })
              )}
            </div>
            <div>
              <h4 className="mb-1 text-xs font-semibold uppercase tracking-wide text-yellow-700 dark:text-yellow-300">New tags</h4>
              {renderTagList(
                newTags,
                "No new tags will be created.",
                "bg-yellow-100 text-yellow-900 dark:bg-yellow-900 dark:text-yellow-100",
                (tag) => ({
                  label: "Remove",
                  onClick: () => onTagDeletion(suggestion.id, tag),
                  className: "bg-yellow-200 text-yellow-900 hover:bg-red-100 hover:text-red-700 dark:bg-yellow-800 dark:text-yellow-100 dark:hover:bg-red-900 dark:hover:text-red-100",
                })
              )}
            </div>
            <div>
              <h4 className="mb-1 text-xs font-semibold uppercase tracking-wide text-red-700 dark:text-red-300">Removed tags</h4>
              {renderTagList(
                removedTags,
                "No existing tags will be removed.",
                "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-100",
                (tag) => ({
                  label: "Keep",
                  onClick: () => onTagRestore(suggestion.id, tag),
                  className: "bg-red-200 text-red-800 no-underline hover:bg-green-100 hover:text-green-700 dark:bg-red-800 dark:text-red-100 dark:hover:bg-green-900 dark:hover:text-green-100",
                })
              )}
            </div>
          </div>
          <div className="mt-3">
            <span className="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-300">Add another tag</span>
          <ReactTags
            selected={[]}
            suggestions={sortedAvailableTags.map((tag) => ({
              id: tag.id,
              name: tag.name,
              label: tag.name,
              value: tag.id,
            }))}
            onAdd={(tag) =>
              onTagAddition(suggestion.id, {
                id: String(tag.label),
                name: String(tag.label),
              })
            }
            onDelete={() => undefined}
            allowNew={true}
            placeholderText="Add a tag"
            classNames={{
              root: "react-tags dark:bg-gray-800",
              rootIsActive: "is-active",
              rootIsDisabled: "is-disabled",
              rootIsInvalid: "is-invalid",
              label: "react-tags__label",
              tagList: "react-tags__list",
              tagListItem: "react-tags__list-item",
              tag: "react-tags__tag dark:bg-blue-900 dark:text-blue-200",
              tagName: "react-tags__tag-name",
              comboBox: "react-tags__combobox dark:bg-gray-700 dark:text-gray-200",
              input: "react-tags__combobox-input dark:bg-gray-700 dark:text-gray-200",
              listBox: "react-tags__listbox dark:bg-gray-700 dark:text-gray-200",
              option: "react-tags__listbox-option dark:bg-gray-700 dark:text-gray-200 hover:bg-blue-500 dark:hover:bg-blue-800",
              optionIsActive: "is-active",
              highlight: "react-tags__highlight dark:bg-gray-800",
            }}
          />
          </div>
        </div>
        <div className="mt-4">
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
            Suggested Correspondent
          </label>
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            Current: {originalValue(document.correspondent)}
          </p>
          <input
            type="text"
            value={suggestion.suggested_correspondent || ""}
            onChange={(e) => onCorrespondentChange(suggestion.id, e.target.value)}
            className="w-full border border-gray-300 dark:border-gray-600 rounded px-2 py-1 mt-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-gray-200"
            placeholder="Correspondent"
          />
        </div>
        <div className="mt-4">
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
            Suggested Document Type
          </label>
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            Current: {originalValue(document.document_type_name)}
          </p>
          {suggestedDocumentType && (
            <div className="mt-2">
              <span
                className={`inline-flex rounded-full px-2.5 py-1 text-xs font-medium ${
                  suggestedDocumentTypeExists
                    ? "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-100"
                    : createNewDocumentTypesEnabled
                      ? "bg-yellow-100 text-yellow-900 dark:bg-yellow-900 dark:text-yellow-100"
                      : "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-100"
                }`}
              >
                {suggestedDocumentTypeExists
                  ? "Existing document type"
                  : createNewDocumentTypesEnabled
                    ? "New document type will be created"
                    : "Unknown document type will be skipped"}
              </span>
            </div>
          )}
          <input
            type="search"
            list={`document-types-${suggestion.id}`}
            value={suggestion.suggested_document_type || ""}
            onChange={(e) => onDocumentTypeChange(suggestion.id, e.target.value)}
            className="w-full border border-gray-300 dark:border-gray-600 rounded px-2 py-1 mt-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-gray-200"
            placeholder="Document Type"
          />
          <datalist id={`document-types-${suggestion.id}`}>
            {sortedAvailableDocumentTypes.map((documentType) => (
              <option key={documentType.id} value={documentType.name} />
            ))}
          </datalist>
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            Choose an existing document type. {createNewDocumentTypesEnabled ? "New values are allowed after review." : "New values are disabled and will not be created."}
          </p>
        </div>
        <div className="mt-4">
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
            Suggested Created Date
          </label>
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            Current: {originalValue(document.created_date)}
          </p>
          <input
            type="text"
            value={suggestion.suggested_created_date || ""}
            onChange={(e) => onCreatedDateChange(suggestion.id, e.target.value)}
            className="w-full border border-gray-300 dark:border-gray-600 rounded px-2 py-1 mt-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-gray-200"
            placeholder="Created Date"
          />
        </div>
        {suggestion.suggested_custom_fields && suggestion.suggested_custom_fields.length > 0 && (
          <div className="mt-4">
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
              Suggested Custom Fields
            </label>
            <div className="mt-2 space-y-2">
              {suggestion.suggested_custom_fields?.map((field) => (
                <div key={field.id} className="flex items-center">
                  <input
                    type="checkbox"
                    id={`custom-field-${suggestion.id}-${field.id}`}
                    checked={field.isSelected}
                    onChange={() => onCustomFieldSuggestionToggle(suggestion.id, field.id)}
                    className="w-4 h-4 mr-2"
                  />
                  <label htmlFor={`custom-field-${suggestion.id}-${field.id}`} className="text-sm">
                    <span className="font-semibold">{field.name}:</span> {String(field.value)}
                  </label>
                </div>
              )) || []}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};

export default SuggestionCard;
