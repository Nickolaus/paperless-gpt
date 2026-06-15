import classNames from "classnames";
import React from "react";
import { ReactTags } from "react-tag-autocomplete";
import {
  DocumentSuggestion,
  DocumentTypeOption,
  TagOption,
} from "../../DocumentProcessor";
import {
  FieldKey,
  SuggestionEditHandlers,
  fieldChanged,
  fieldLabels,
  originalValue,
} from "./fields";

interface FieldRowProps {
  suggestion: DocumentSuggestion;
  fieldKey: FieldKey;
  excluded: boolean;
  onToggleField: (docId: number, key: FieldKey) => void;
  children: React.ReactNode;
}

/**
 * One field as a diff: original value (when it changes) above the editable
 * suggested value, with a per-field apply toggle.
 */
const FieldRow: React.FC<FieldRowProps> = ({
  suggestion,
  fieldKey,
  excluded,
  onToggleField,
  children,
}) => {
  const changed = fieldChanged(suggestion, fieldKey);
  const original = originalValue(suggestion, fieldKey);
  const originalText = Array.isArray(original)
    ? original.join(", ")
    : original;
  const label = fieldLabels[fieldKey];

  return (
    <div className="py-2.5">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-medium text-muted">{label}</span>
        {changed && (
          <label className="inline-flex cursor-pointer items-center gap-1.5 text-xs text-muted">
            <input
              type="checkbox"
              checked={!excluded}
              onChange={() => onToggleField(suggestion.id, fieldKey)}
              aria-label={`Apply suggested ${label.toLowerCase()}`}
              className="h-3.5 w-3.5 cursor-pointer rounded accent-primary"
            />
            Apply
          </label>
        )}
      </div>

      {changed && originalText && (
        <p className="mt-1 flex items-baseline gap-1.5 text-sm">
          <span className="select-none text-neg" aria-hidden="true">
            −
          </span>
          <span className="min-w-0 break-words text-muted line-through">
            <span className="sr-only">Current value: </span>
            {originalText}
          </span>
        </p>
      )}

      <div
        className={classNames(
          "mt-1 flex items-start gap-1.5",
          excluded && "opacity-50"
        )}
      >
        {changed && (
          <span
            className="select-none pt-1.5 text-sm text-pos"
            aria-hidden="true"
          >
            +
          </span>
        )}
        <div className="min-w-0 flex-1">{children}</div>
      </div>
    </div>
  );
};

const inputClasses =
  "w-full rounded-md border border-line bg-surface px-2.5 py-1.5 text-sm text-ink disabled:cursor-not-allowed";

interface SuggestionFieldsProps {
  suggestion: DocumentSuggestion;
  availableTags: TagOption[];
  tagSelectionMode: "all" | "applicable";
  tagDerivedParents: boolean;
  createNewTagsEnabled: boolean;
  availableDocumentTypes: DocumentTypeOption[];
  createNewDocumentTypesEnabled: boolean;
  excluded: Set<FieldKey>;
  onToggleField: (docId: number, key: FieldKey) => void;
  handlers: SuggestionEditHandlers;
  disabled?: boolean;
}

const tagEquals = (left: string, right: string) =>
  left.localeCompare(right, undefined, { sensitivity: "accent" }) === 0;

/** The editable field list of one suggestion, shared by card and focus view. */
const SuggestionFields: React.FC<SuggestionFieldsProps> = ({
  suggestion,
  availableTags,
  tagSelectionMode,
  createNewTagsEnabled,
  availableDocumentTypes,
  createNewDocumentTypesEnabled,
  excluded,
  onToggleField,
  handlers,
  disabled,
}) => {
  const selectableTags = availableTags.filter(
    (tag) => tagSelectionMode !== "applicable" || tag.is_applicable
  );
  const sortedAvailableTags = [...selectableTags].sort((a, b) =>
    (a.path || a.name).localeCompare(b.path || b.name)
  );
  const sortedAvailableDocumentTypes = [...availableDocumentTypes].sort(
    (a, b) => a.name.localeCompare(b.name)
  );
  const [isCreatingDocumentType, setIsCreatingDocumentType] =
    React.useState(false);
  const suggestedDocumentType = suggestion.suggested_document_type?.trim() || "";
  const suggestedDocumentTypeExists = suggestedDocumentType
    ? sortedAvailableDocumentTypes.some((documentType) =>
        tagEquals(documentType.name, suggestedDocumentType)
      )
    : false;
  const documentTypeSelectValue = suggestedDocumentType
    ? suggestedDocumentTypeExists
      ? sortedAvailableDocumentTypes.find((documentType) =>
          tagEquals(documentType.name, suggestedDocumentType)
        )?.name || ""
      : createNewDocumentTypesEnabled
        ? "__new__"
        : "__unknown__"
    : isCreatingDocumentType
      ? "__new__"
      : "";

  return (
    <div className="divide-y divide-line">
      <FieldRow
        suggestion={suggestion}
        fieldKey="title"
        excluded={excluded.has("title")}
        onToggleField={onToggleField}
      >
        <input
          type="text"
          value={suggestion.suggested_title || ""}
          onChange={(e) => handlers.onTitleChange(suggestion.id, e.target.value)}
          disabled={disabled || excluded.has("title")}
          aria-label="Suggested title"
          className={inputClasses}
        />
      </FieldRow>

      <FieldRow
        suggestion={suggestion}
        fieldKey="tags"
        excluded={excluded.has("tags")}
        onToggleField={onToggleField}
      >
        <ReactTags
          selected={
            suggestion.suggested_tags?.map((tag, index) => ({
              id: index.toString(),
              name: tag,
              label: tag,
              value: index.toString(),
            })) || []
          }
          suggestions={sortedAvailableTags.map((tag) => ({
            id: tag.id,
            name: tag.name,
            label: tag.path || tag.name,
            value: tag.id,
          }))}
          onAdd={(tag) => {
            const matched = sortedAvailableTags.find(
              (candidate) => candidate.id === String(tag.value)
            );
            handlers.onTagAddition(suggestion.id, {
              id: String(tag.value ?? tag.label),
              name: matched?.name || String(tag.label),
            });
          }}
          onDelete={(index) => handlers.onTagDeletion(suggestion.id, index)}
          allowNew={createNewTagsEnabled}
          placeholderText="Add a tag"
          labelText="Suggested tags"
          isDisabled={disabled || excluded.has("tags")}
        />
      </FieldRow>

      <FieldRow
        suggestion={suggestion}
        fieldKey="correspondent"
        excluded={excluded.has("correspondent")}
        onToggleField={onToggleField}
      >
        <input
          type="text"
          value={suggestion.suggested_correspondent || ""}
          onChange={(e) =>
            handlers.onCorrespondentChange(suggestion.id, e.target.value)
          }
          disabled={disabled || excluded.has("correspondent")}
          aria-label="Suggested correspondent"
          placeholder="Correspondent"
          className={inputClasses}
        />
      </FieldRow>

      <FieldRow
        suggestion={suggestion}
        fieldKey="document_type"
        excluded={excluded.has("document_type")}
        onToggleField={onToggleField}
      >
        <select
          value={documentTypeSelectValue}
          onChange={(e) => {
            const value = e.target.value;
            if (value === "__new__") {
              setIsCreatingDocumentType(true);
              handlers.onDocumentTypeChange(
                suggestion.id,
                suggestedDocumentTypeExists ? "" : suggestedDocumentType
              );
            } else if (value === "__unknown__") {
              setIsCreatingDocumentType(false);
              handlers.onDocumentTypeChange(suggestion.id, suggestedDocumentType);
            } else {
              setIsCreatingDocumentType(false);
              handlers.onDocumentTypeChange(suggestion.id, value);
            }
          }}
          disabled={disabled || excluded.has("document_type")}
          aria-label="Suggested document type"
          className={inputClasses}
        >
          <option value="">No document type</option>
          {sortedAvailableDocumentTypes.map((documentType) => (
            <option key={documentType.id} value={documentType.name}>
              {documentType.name}
            </option>
          ))}
          {createNewDocumentTypesEnabled && (
            <option value="__new__">New document type...</option>
          )}
          {!createNewDocumentTypesEnabled &&
            suggestedDocumentType &&
            !suggestedDocumentTypeExists && (
              <option value="__unknown__">{suggestedDocumentType}</option>
            )}
        </select>
        {createNewDocumentTypesEnabled && documentTypeSelectValue === "__new__" && (
          <input
            type="text"
            value={suggestedDocumentType}
            onChange={(e) =>
              handlers.onDocumentTypeChange(suggestion.id, e.target.value)
            }
            disabled={disabled || excluded.has("document_type")}
            aria-label="New document type name"
            placeholder="New document type"
            className={classNames(inputClasses, "mt-1.5")}
          />
        )}
      </FieldRow>

      <FieldRow
        suggestion={suggestion}
        fieldKey="created_date"
        excluded={excluded.has("created_date")}
        onToggleField={onToggleField}
      >
        <input
          type="text"
          value={suggestion.suggested_created_date || ""}
          onChange={(e) =>
            handlers.onCreatedDateChange(suggestion.id, e.target.value)
          }
          disabled={disabled || excluded.has("created_date")}
          aria-label="Suggested created date"
          placeholder="YYYY-MM-DD"
          inputMode="numeric"
          pattern="\d{4}-\d{2}-\d{2}"
          title="Use the format YYYY-MM-DD"
          className={inputClasses}
        />
      </FieldRow>

      {suggestion.suggested_custom_fields &&
        suggestion.suggested_custom_fields.length > 0 && (
          <div className="py-2.5">
            <span className="text-xs font-medium text-muted">
              Custom fields
            </span>
            <div className="mt-2 space-y-1.5">
              {suggestion.suggested_custom_fields.map((field) => (
                <div key={field.id} className="flex items-start gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={field.isSelected}
                    onChange={() =>
                      handlers.onCustomFieldSuggestionToggle(
                        suggestion.id,
                        field.id
                      )
                    }
                    disabled={disabled}
                    aria-label={`Apply suggested ${field.name}`}
                    className="mt-2 h-4 w-4 cursor-pointer rounded accent-primary"
                  />
                  <label className="min-w-0 flex-1">
                    <span className="text-xs font-medium text-muted">
                      {field.name}
                    </span>
                    <input
                      type="text"
                      value={String(field.value ?? "")}
                      onChange={(e) =>
                        handlers.onCustomFieldSuggestionValueChange(
                          suggestion.id,
                          field.id,
                          e.target.value
                        )
                      }
                      disabled={disabled}
                      className={classNames(inputClasses, "mt-1")}
                    />
                  </label>
                </div>
              ))}
            </div>
          </div>
        )}
    </div>
  );
};

export default SuggestionFields;
