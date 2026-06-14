import { Disclosure, DisclosureButton, DisclosurePanel, RadioGroup } from "@headlessui/react";
import axios from "axios";
import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import "react-tag-autocomplete/example/src/styles.css";
import ArrowPathIcon from "@heroicons/react/24/outline/ArrowPathIcon";
import CheckCircleIcon from "@heroicons/react/24/outline/CheckCircleIcon";
import ChevronDownIcon from "@heroicons/react/24/outline/ChevronDownIcon";
import StopIcon from "@heroicons/react/24/outline/StopIcon";
import DocumentsToProcess from "./components/DocumentsToProcess";
import NoDocuments from "./components/NoDocuments";
import SuccessModal from "./components/SuccessModal";
import SuggestionsReview from "./components/SuggestionsReview";
import { OCRJobStatus, mapJobStatus } from "./ocrStatus";

export interface Document {
  id: number;
  title: string;
  content: string;
  tags: string[];
  correspondent: string;
  created_date?: string;
  original_file_name?: string;
  document_type_name?: string;
  custom_fields?: CustomFieldResponse[];
}

export interface GenerateSuggestionsRequest {
  documents: Document[];
  generate_titles?: boolean;
  generate_tags?: boolean;
  generate_correspondents?: boolean;
  generate_document_types?: boolean;
  generate_created_date?: boolean;
  generate_custom_fields?: boolean;
  selected_custom_field_ids?: number[];
  custom_field_write_mode?: string;
}

export interface CustomFieldSuggestion {
  id: number;
  value: unknown;
  name: string;
  isSelected: boolean;
}

export interface CustomFieldResponse {
  field: number;
  value: unknown;
  name?: string;
}

export interface DocumentSuggestion {
  id: number;
  original_document: Document;
  suggested_title?: string;
  suggested_tags?: string[];
  suggested_content?: string;
  suggested_correspondent?: string;
  suggested_document_type?: string;
  suggested_created_date?: string;
  suggested_custom_fields?: CustomFieldSuggestion[];
  field_errors?: Record<string, string>;
  keep_original_tags?: boolean;
  remove_tags?: string[];
  add_tags?: string[];
  custom_fields_write_mode?: string;
}

export interface TagOption {
  id: string;
  name: string;
}

export interface DocumentTypeOption {
  id: number;
  name: string;
}

interface DocumentTypesResponse {
  document_types: DocumentTypeOption[];
  create_new_document_types: boolean;
}

interface SuggestionJobResponse {
  job_id: string;
  status: "pending" | "in_progress" | "completed" | "failed" | "cancelled";
  documents_done: number;
  total_documents: number;
  current_document_id?: number;
  result?: DocumentSuggestion[];
  error?: string;
}

interface CustomField {
  id: number;
  name: string;
  data_type: string;
}

interface OCRPageResult {
  text: string;
  ocrLimitHit: boolean;
  generationInfo?: Record<string, unknown>;
}

interface OCRCombinedResult {
  combinedText: string;
  perPageResults: OCRPageResult[];
}

interface OCRDocumentResult {
  documentId: number;
  document: Document;
  combinedText: string;
  perPageResults: OCRPageResult[];
  saved: boolean;
  error?: string;
}

type WorkflowMode = "suggestions_only" | "ocr_then_suggestions" | "ocr_only";
type WorkflowStep = "select" | "ocr" | "suggestions" | "review";
type SuggestionPreset = "core" | "classification" | "everything" | "custom";

interface PersistedWorkflowState {
  selectedDocuments: number[];
  workflowMode: WorkflowMode;
  activeStep: WorkflowStep;
}

const activeSuggestionJobStorageKey = "paperless-gpt-active-suggestion-job-id";
const activeOCRJobStorageKey = "paperless-gpt-active-ocr-job-id";
const activeOCRDocumentStorageKey = "paperless-gpt-active-ocr-document-id";
const activeOCRQueueStorageKey = "paperless-gpt-active-ocr-queue";
const workflowStateStorageKey = "paperless-gpt-manual-workflow-state";
const ocrJobPollIntervalMs = 1000;
const ocrJobPollRetryMaxDelayMs = 10000;
const suggestionJobPollIntervalMs = 1500;

const workflowModes: Array<{
  id: WorkflowMode;
  title: string;
  description: string;
  requiresOCR?: boolean;
}> = [
  {
    id: "suggestions_only",
    title: "Suggestions only",
    description: "Use the current Paperless text and generate metadata suggestions.",
  },
  {
    id: "ocr_then_suggestions",
    title: "OCR then suggestions",
    description: "Improve text first, review and save it, then generate metadata suggestions.",
    requiresOCR: true,
  },
  {
    id: "ocr_only",
    title: "OCR only",
    description: "Run OCR rescue or enrichment and stop after reviewing the extracted text.",
    requiresOCR: true,
  },
];

const suggestionPresetOptions: Array<{
  id: SuggestionPreset;
  title: string;
  description: string;
}> = [
  {
    id: "core",
    title: "Core metadata",
    description: "Title, correspondent, document type, and created date.",
  },
  {
    id: "classification",
    title: "Classification",
    description: "Tags and document type.",
  },
  {
    id: "everything",
    title: "Everything",
    description: "All enabled suggestion fields, including custom fields.",
  },
  {
    id: "custom",
    title: "Custom",
    description: "Choose individual fields manually.",
  },
];

const tagEquals = (left: string, right: string) => left.localeCompare(right, undefined, { sensitivity: "accent" }) === 0;

const includesTag = (tags: string[] | undefined, tag: string) =>
  Boolean(tags?.some((candidate) => tagEquals(candidate, tag)));

const uniqueTags = (tags: string[]) =>
  tags.reduce<string[]>((unique, tag) => {
    const trimmedTag = tag.trim();
    if (trimmedTag && !includesTag(unique, trimmedTag)) {
      unique.push(trimmedTag);
    }
    return unique;
  }, []);

const buildSelectedTags = (originalTags: string[], addTags: string[], removeTags: string[]) =>
  uniqueTags([...originalTags, ...addTags]).filter((tag) => !includesTag(removeTags, tag));

const loadPersistedWorkflowState = (): Partial<PersistedWorkflowState> => {
  try {
    const raw = localStorage.getItem(workflowStateStorageKey);
    if (!raw) return {};
    return JSON.parse(raw) as PersistedWorkflowState;
  } catch {
    return {};
  }
};

const loadPersistedNumberArray = (key: string): number[] => {
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((value) => Number.isInteger(value)) : [];
  } catch {
    return [];
  }
};

const loadInitialWorkflowStep = (persistedStep?: WorkflowStep): WorkflowStep => {
  if (persistedStep === "suggestions") {
    return localStorage.getItem(activeSuggestionJobStorageKey) ? "suggestions" : "select";
  }

  if (persistedStep === "ocr") {
    return localStorage.getItem(activeOCRJobStorageKey) ? "ocr" : "select";
  }

  if (persistedStep === "review") {
    return "select";
  }

  return persistedStep || "select";
};

const parseOCRResult = (result: unknown): OCRCombinedResult => {
  if (typeof result !== "string") {
    return { combinedText: "", perPageResults: [] };
  }

  try {
    const parsed = JSON.parse(result) as OCRCombinedResult;
    return {
      combinedText: parsed.combinedText || result,
      perPageResults: parsed.perPageResults || [],
    };
  } catch {
    return { combinedText: result, perPageResults: [] };
  }
};

const formatGenerationInfo = (value: unknown) =>
  typeof value === "object" ? JSON.stringify(value) : String(value);

const DocumentProcessor: React.FC = () => {
  const persistedWorkflow = useMemo(loadPersistedWorkflowState, []);
  const [documents, setDocuments] = useState<Document[]>([]);
  const [selectedDocuments, setSelectedDocuments] = useState<number[]>(
    persistedWorkflow.selectedDocuments || []
  );
  const [suggestions, setSuggestions] = useState<DocumentSuggestion[]>([]);
  const [availableTags, setAvailableTags] = useState<TagOption[]>([]);
  const [availableDocumentTypes, setAvailableDocumentTypes] = useState<DocumentTypeOption[]>([]);
  const [createNewDocumentTypesEnabled, setCreateNewDocumentTypesEnabled] = useState(false);
  const [allCustomFields, setAllCustomFields] = useState<CustomField[]>([]);
  const [loading, setLoading] = useState(true);
  const [processing, setProcessing] = useState(false);
  const [suggestionJobId, setSuggestionJobId] = useState<string>(
    () => localStorage.getItem(activeSuggestionJobStorageKey) || ""
  );
  const [suggestionJobStatus, setSuggestionJobStatus] = useState<SuggestionJobResponse["status"] | "idle">("idle");
  const [documentsDone, setDocumentsDone] = useState(0);
  const [totalDocuments, setTotalDocuments] = useState(0);
  const [currentDocumentId, setCurrentDocumentId] = useState<number | null>(null);
  const [updating, setUpdating] = useState(false);
  const [isSuccessModalOpen, setIsSuccessModalOpen] = useState(false);
  const [filterTag, setFilterTag] = useState<string | null>(null);
  const [ocrEnabled, setOcrEnabled] = useState(false);
  const [workflowMode, setWorkflowMode] = useState<WorkflowMode>(
    persistedWorkflow.workflowMode || "suggestions_only"
  );
  const [activeStep, setActiveStep] = useState<WorkflowStep>(
    () => loadInitialWorkflowStep(persistedWorkflow.activeStep)
  );
  const [suggestionPreset, setSuggestionPreset] = useState<SuggestionPreset>("everything");
  const [generateTitles, setGenerateTitles] = useState(true);
  const [generateTags, setGenerateTags] = useState(true);
  const [generateCorrespondents, setGenerateCorrespondents] = useState(true);
  const [generateDocumentTypes, setGenerateDocumentTypes] = useState(true);
  const [generateCreatedDate, setGenerateCreatedDate] = useState(true);
  const [generateCustomFields, setGenerateCustomFields] = useState(true);
  const [ocrJobId, setOcrJobId] = useState<string>(() => localStorage.getItem(activeOCRJobStorageKey) || "");
  const [ocrJobStatus, setOcrJobStatus] = useState<OCRJobStatus>(ocrJobId ? "pending" : "idle");
  const [ocrQueue, setOcrQueue] = useState<number[]>(() => loadPersistedNumberArray(activeOCRQueueStorageKey));
  const [currentOCRDocumentId, setCurrentOCRDocumentId] = useState<number | null>(
    () => Number(localStorage.getItem(activeOCRDocumentStorageKey)) || null
  );
  const [ocrPagesDone, setOcrPagesDone] = useState(0);
  const [ocrTotalPages, setOcrTotalPages] = useState<number | null>(null);
  const [ocrResults, setOCRResults] = useState<Record<number, OCRDocumentResult>>({});
  const [ocrSkipAccepted, setOCRSkipAccepted] = useState(false);
  const [savingOCRDocumentId, setSavingOCRDocumentId] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const suggestionPollTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const ocrPollTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const ocrPollFailureCountRef = useRef(0);

  const workflowModeUsesOCR = workflowMode === "ocr_then_suggestions" || workflowMode === "ocr_only";
  const workflowModeUsesSuggestions = workflowMode === "suggestions_only" || workflowMode === "ocr_then_suggestions";
  const allSelectedOCRComplete =
    selectedDocuments.length > 0 &&
    selectedDocuments.every((documentId) => Boolean(ocrResults[documentId]?.combinedText || ocrResults[documentId]?.error));
  const allSelectedOCRReadyForSuggestions =
    allSelectedOCRComplete &&
    (ocrSkipAccepted || selectedDocuments.every((documentId) => ocrResults[documentId]?.saved || ocrResults[documentId]?.error));
  const hasUnsavedOCRContent = selectedDocuments.some((documentId) => {
    const result = ocrResults[documentId];
    return Boolean(result?.combinedText && !result.saved);
  });
  const unsavedOCRContentCount = selectedDocuments.filter((documentId) => {
    const result = ocrResults[documentId];
    return Boolean(result?.combinedText && !result.saved);
  }).length;

  useEffect(() => {
    localStorage.setItem(
      workflowStateStorageKey,
      JSON.stringify({ selectedDocuments, workflowMode, activeStep })
    );
  }, [selectedDocuments, workflowMode, activeStep]);

  const processSuggestionResults = useCallback((data: DocumentSuggestion[]) => {
    const customFieldMap = new Map((allCustomFields || []).map((cf) => [cf.id, cf.name]));
    const processedSuggestions = data.map((suggestion) => {
      const originalTags = suggestion.original_document.tags || [];
      const suggestedTags = suggestion.suggested_tags || [];
      const existingAddTags = suggestion.add_tags || [];
      const suggestedAddTags = suggestedTags.filter((tag) => !includesTag(originalTags, tag));
      const addTags = uniqueTags([...existingAddTags, ...suggestedAddTags]);
      const removeTags = uniqueTags(suggestion.remove_tags || []);

      return {
        ...suggestion,
        keep_original_tags: true,
        add_tags: addTags,
        remove_tags: removeTags,
        suggested_tags: buildSelectedTags(originalTags, addTags, removeTags),
        suggested_custom_fields: suggestion.suggested_custom_fields?.map((cf) => ({
          ...cf,
          name: customFieldMap.get(cf.id) ?? cf.name ?? "Unknown Field",
          isSelected: true,
        })),
      };
    });

    setSuggestions(processedSuggestions);
    setActiveStep("review");
  }, [allCustomFields]);

  const clearActiveSuggestionJob = useCallback(() => {
    if (suggestionPollTimeoutRef.current) {
      clearTimeout(suggestionPollTimeoutRef.current);
      suggestionPollTimeoutRef.current = null;
    }
    localStorage.removeItem(activeSuggestionJobStorageKey);
    setSuggestionJobId("");
    setProcessing(false);
    setCurrentDocumentId(null);
  }, []);

  const fetchInitialData = useCallback(async () => {
    try {
      const [filterTagRes, documentsRes, tagsRes, documentTypesRes, customFieldsRes, ocrEnabledRes] = await Promise.all([
        axios.get<{ tag: string }>("./api/filter-tag"),
        axios.get<Document[]>("./api/documents"),
        axios.get<Record<string, number>>("./api/tags"),
        axios.get<DocumentTypesResponse>("./api/document_types"),
        axios.get<CustomField[]>("./api/custom_fields"),
        axios.get<{ enabled: boolean }>("./api/experimental/ocr"),
      ]);

      setFilterTag(filterTagRes.data.tag);
      setAllCustomFields(customFieldsRes.data || []);
      setDocuments(documentsRes.data);
      setSelectedDocuments((previous) =>
        previous.length > 0 ? previous.filter((id) => documentsRes.data.some((doc) => doc.id === id)) : documentsRes.data.map((doc) => doc.id)
      );
      setAvailableTags(Object.keys(tagsRes.data).map((tag) => ({ id: tag, name: tag })));
      setAvailableDocumentTypes(documentTypesRes.data.document_types || []);
      setCreateNewDocumentTypesEnabled(Boolean(documentTypesRes.data.create_new_document_types));
      setOcrEnabled(ocrEnabledRes.data.enabled);
      if (!ocrEnabledRes.data.enabled) {
        setWorkflowMode((current) => current === "suggestions_only" ? current : "suggestions_only");
      }
    } catch (err) {
      console.error("Error fetching initial data:", err);
      setError("Failed to fetch initial data.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchInitialData();
  }, [fetchInitialData]);

  const pollSuggestionJob = useCallback(async (jobId: string) => {
    if (!jobId) return;

    try {
      const { data } = await axios.get<SuggestionJobResponse>(`./api/jobs/suggestions/${jobId}`);
      setSuggestionJobStatus(data.status);
      setDocumentsDone(data.documents_done || 0);
      setTotalDocuments(data.total_documents || 0);
      setCurrentDocumentId(data.current_document_id || null);

      if (data.status === "completed") {
        processSuggestionResults(data.result || []);
        clearActiveSuggestionJob();
      } else if (data.status === "failed" || data.status === "cancelled") {
        setError(data.error || `Suggestion job ${data.status}.`);
        clearActiveSuggestionJob();
      } else {
        setProcessing(true);
        setActiveStep("suggestions");
        suggestionPollTimeoutRef.current = setTimeout(() => pollSuggestionJob(jobId), suggestionJobPollIntervalMs);
      }
    } catch (err) {
      console.error("Error checking suggestion job status:", err);
      if (axios.isAxiosError(err) && (err.response?.status === 404 || err.response?.status === 410)) {
        setError("Suggestion job is no longer available.");
        clearActiveSuggestionJob();
        return;
      }
      setError("Failed to check suggestion job status. Retrying...");
      setProcessing(true);
      suggestionPollTimeoutRef.current = setTimeout(() => pollSuggestionJob(jobId), suggestionJobPollIntervalMs);
    }
  }, [clearActiveSuggestionJob, processSuggestionResults]);

  useEffect(() => {
    if (loading) return;

    if (suggestionJobId) {
      setProcessing(true);
      setActiveStep("suggestions");
      pollSuggestionJob(suggestionJobId);
    }

    return () => {
      if (suggestionPollTimeoutRef.current) {
        clearTimeout(suggestionPollTimeoutRef.current);
        suggestionPollTimeoutRef.current = null;
      }
    };
  }, [loading, pollSuggestionJob, suggestionJobId]);

  const handleSelectDocument = (documentId: number) => {
    setSelectedDocuments((previous) =>
      previous.includes(documentId)
        ? previous.filter((id) => id !== documentId)
        : [...previous, documentId]
    );
  };

  const reloadDocuments = async () => {
    setLoading(true);
    setError(null);
    try {
      const { data } = await axios.get<Document[]>("./api/documents");
      setDocuments(data);
      setSelectedDocuments((previous) =>
        previous.length > 0 ? previous.filter((id) => data.some((doc) => doc.id === id)) : data.map((doc) => doc.id)
      );
    } catch (err) {
      console.error("Error reloading documents:", err);
      setError("Failed to reload documents.");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (documents.length === 0) {
      const interval = setInterval(async () => {
        setError(null);
        try {
          const { data } = await axios.get<Document[]>("./api/documents");
          setDocuments(data);
          setSelectedDocuments((previous) => previous.length > 0 ? previous : data.map((doc) => doc.id));
        } catch (err) {
          console.error("Error reloading documents:", err);
          setError("Failed to reload documents.");
        }
      }, 500);
      return () => clearInterval(interval);
    }
  }, [documents]);

  const applySuggestionPreset = (preset: SuggestionPreset) => {
    setSuggestionPreset(preset);
    if (preset === "core") {
      setGenerateTitles(true);
      setGenerateTags(false);
      setGenerateCorrespondents(true);
      setGenerateDocumentTypes(true);
      setGenerateCreatedDate(true);
      setGenerateCustomFields(false);
    } else if (preset === "classification") {
      setGenerateTitles(false);
      setGenerateTags(true);
      setGenerateCorrespondents(false);
      setGenerateDocumentTypes(true);
      setGenerateCreatedDate(false);
      setGenerateCustomFields(false);
    } else if (preset === "everything") {
      setGenerateTitles(true);
      setGenerateTags(true);
      setGenerateCorrespondents(true);
      setGenerateDocumentTypes(true);
      setGenerateCreatedDate(true);
      setGenerateCustomFields(true);
    }
  };

  const markCustomPreset = (setter: React.Dispatch<React.SetStateAction<boolean>>) => (value: boolean) => {
    setSuggestionPreset("custom");
    setter(value);
  };

  const startSuggestions = async () => {
    const documentsToProcess = documents.filter((doc) => selectedDocuments.includes(doc.id));
    if (documentsToProcess.length === 0) {
      setError("Select at least one document to process.");
      return;
    }

    setProcessing(true);
    setActiveStep("suggestions");
    setError(null);
    setDocumentsDone(0);
    setTotalDocuments(documentsToProcess.length);
    setCurrentDocumentId(null);
    try {
      const requestPayload: GenerateSuggestionsRequest = {
        documents: documentsToProcess,
        generate_titles: generateTitles,
        generate_tags: generateTags,
        generate_correspondents: generateCorrespondents,
        generate_document_types: generateDocumentTypes,
        generate_created_date: generateCreatedDate,
        generate_custom_fields: generateCustomFields,
      };

      const { data } = await axios.post<{ job_id: string }>("./api/jobs/suggestions", requestPayload);
      localStorage.setItem(activeSuggestionJobStorageKey, data.job_id);
      setSuggestionJobId(data.job_id);
      setSuggestionJobStatus("pending");
    } catch (err) {
      console.error("Error generating suggestions:", err);
      setError("Failed to submit suggestion job.");
      setProcessing(false);
    }
  };

  const fetchOCRPages = async (documentId: number) => {
    try {
      const response = await axios.get<{ pages: OCRPageResult[] }>(`./api/documents/${documentId}/ocr_pages`);
      return response.data.pages || [];
    } catch (err) {
      console.error("Error fetching per-page OCR results:", err);
      return [];
    }
  };

  const clearActiveOCRJob = useCallback(() => {
    if (ocrPollTimeoutRef.current) {
      clearTimeout(ocrPollTimeoutRef.current);
      ocrPollTimeoutRef.current = null;
    }
    localStorage.removeItem(activeOCRJobStorageKey);
    localStorage.removeItem(activeOCRDocumentStorageKey);
    localStorage.removeItem(activeOCRQueueStorageKey);
    setOcrJobId("");
    setCurrentOCRDocumentId(null);
    setOcrQueue([]);
    setProcessing(false);
  }, []);

  const startOCRForDocument = useCallback(async (documentId: number, remainingQueue: number[]) => {
    setError(null);
    setProcessing(true);
    setActiveStep("ocr");
    setCurrentOCRDocumentId(documentId);
    setOcrPagesDone(0);
    setOcrTotalPages(null);
    setOcrJobStatus("pending");

    try {
      const documentResponse = await axios.get<Document>(`./api/documents/${documentId}`);
      const response = await axios.post<{ job_id: string }>(`./api/documents/${documentId}/ocr`);
      setOCRResults((previous) => ({
        ...previous,
        [documentId]: {
          documentId,
          document: documentResponse.data,
          combinedText: "",
          perPageResults: [],
          saved: false,
        },
      }));
      setOcrQueue(remainingQueue);
      setOcrJobId(response.data.job_id);
      localStorage.setItem(activeOCRJobStorageKey, response.data.job_id);
      localStorage.setItem(activeOCRDocumentStorageKey, String(documentId));
      localStorage.setItem(activeOCRQueueStorageKey, JSON.stringify(remainingQueue));
    } catch (err) {
      console.error("Error submitting OCR job:", err);
      setError(`Failed to submit OCR job for document ${documentId}.`);
      setOCRResults((previous) => ({
        ...previous,
        [documentId]: {
          documentId,
          document: documents.find((doc) => doc.id === documentId) || { id: documentId, title: `Document ${documentId}`, content: "", tags: [], correspondent: "" },
          combinedText: "",
          perPageResults: [],
          saved: false,
          error: "Failed to submit OCR job.",
        },
      }));
      const [, ...nextQueue] = remainingQueue;
      if (nextQueue.length > 0) {
        await startOCRForDocument(nextQueue[0], nextQueue);
      } else {
        clearActiveOCRJob();
      }
    }
  }, [clearActiveOCRJob, documents]);

  const pollOCRJob = useCallback(async (jobId: string, documentId: number) => {
    if (!jobId || !documentId) return;

    try {
      const response = await axios.get(`./api/jobs/ocr/${jobId}`);
      ocrPollFailureCountRef.current = 0;
      const newJobStatus = mapJobStatus(response.data.status);
      setOcrJobStatus(newJobStatus);
      setOcrPagesDone(response.data.pages_done || 0);
      setOcrTotalPages(response.data.total_pages ?? null);

      if (response.data.pages_done > 0) {
        const pages = await fetchOCRPages(documentId);
        setOCRResults((previous) => ({
          ...previous,
          [documentId]: {
            ...(previous[documentId] || {
              documentId,
              document: documents.find((doc) => doc.id === documentId) || { id: documentId, title: `Document ${documentId}`, content: "", tags: [], correspondent: "" },
              combinedText: "",
              saved: false,
            }),
            perPageResults: pages,
          },
        }));
      }

      if (newJobStatus === "completed") {
        const parsedResult = parseOCRResult(response.data.result);
        let resolvedDocument = documents.find((doc) => doc.id === documentId) || {
          id: documentId,
          title: `Document ${documentId}`,
          content: "",
          tags: [],
          correspondent: "",
        };
        try {
          const documentResponse = await axios.get<Document>(`./api/documents/${documentId}`);
          resolvedDocument = documentResponse.data;
        } catch (err) {
          console.error("Error fetching completed OCR document details:", err);
        }
        setOCRResults((previous) => ({
          ...previous,
          [documentId]: {
            ...(previous[documentId] || {
              documentId,
              document: resolvedDocument,
              saved: false,
            }),
            document: resolvedDocument,
            combinedText: parsedResult.combinedText,
            perPageResults: parsedResult.perPageResults,
          },
        }));

        localStorage.removeItem(activeOCRJobStorageKey);
        localStorage.removeItem(activeOCRDocumentStorageKey);
        setOcrJobId("");
        setCurrentOCRDocumentId(null);

        const nextQueue = ocrQueue.filter((id) => id !== documentId);
        if (nextQueue.length > 0) {
          await startOCRForDocument(nextQueue[0], nextQueue);
        } else {
          setProcessing(false);
          setOcrJobStatus("completed");
        }
      } else if (newJobStatus === "failed" || newJobStatus === "cancelled") {
        const errorMessage = response.data.error || `OCR job ${newJobStatus}.`;
        setOCRResults((previous) => ({
          ...previous,
          [documentId]: {
            ...(previous[documentId] || {
              documentId,
              document: documents.find((doc) => doc.id === documentId) || { id: documentId, title: `Document ${documentId}`, content: "", tags: [], correspondent: "" },
              combinedText: "",
              perPageResults: [],
              saved: false,
            }),
            error: errorMessage,
          },
        }));
        localStorage.removeItem(activeOCRJobStorageKey);
        localStorage.removeItem(activeOCRDocumentStorageKey);
        setOcrJobId("");
        setCurrentOCRDocumentId(null);
        const nextQueue = ocrQueue.filter((id) => id !== documentId);
        if (nextQueue.length > 0) {
          await startOCRForDocument(nextQueue[0], nextQueue);
        } else {
          setProcessing(false);
        }
      } else {
        ocrPollTimeoutRef.current = setTimeout(() => pollOCRJob(jobId, documentId), ocrJobPollIntervalMs);
      }
    } catch (err) {
      console.error("Error checking OCR job status:", err);
      ocrPollFailureCountRef.current += 1;
      const retryDelay = Math.min(ocrJobPollIntervalMs * 2 ** ocrPollFailureCountRef.current, ocrJobPollRetryMaxDelayMs);
      setError("Failed to check OCR job status. Retrying...");
      ocrPollTimeoutRef.current = setTimeout(() => pollOCRJob(jobId, documentId), retryDelay);
    }
  }, [documents, ocrQueue, startOCRForDocument]);

  useEffect(() => {
    if (ocrJobId && currentOCRDocumentId) {
      setProcessing(true);
      setActiveStep("ocr");
      pollOCRJob(ocrJobId, currentOCRDocumentId);
    }

    return () => {
      if (ocrPollTimeoutRef.current) {
        clearTimeout(ocrPollTimeoutRef.current);
        ocrPollTimeoutRef.current = null;
      }
    };
  }, [currentOCRDocumentId, ocrJobId, pollOCRJob]);

  const startWorkflow = async () => {
    if (selectedDocuments.length === 0) {
      setError("Select at least one document to process.");
      return;
    }

    setError(null);
    setSuggestions([]);
    setOCRSkipAccepted(false);

    if (workflowModeUsesOCR) {
      setOCRResults({});
      await startOCRForDocument(selectedDocuments[0], selectedDocuments);
      return;
    }

    await startSuggestions();
  };

  const handleStopOCRJob = async () => {
    if (!ocrJobId) return;
    try {
      await axios.post(`./api/ocr/jobs/${ocrJobId}/stop`);
      setOcrJobStatus("cancelled");
      clearActiveOCRJob();
      setError("OCR job cancelled.");
    } catch (err) {
      console.error("Error stopping OCR job:", err);
      setError("Failed to stop OCR job.");
    }
  };

  const handleSaveOCRContent = async (result: OCRDocumentResult) => {
    if (!result.combinedText) return;
    setSavingOCRDocumentId(result.documentId);
    setError(null);
    try {
      await axios.patch("./api/update-documents", [{
        id: result.documentId,
        original_document: result.document,
        suggested_content: result.combinedText,
      } satisfies DocumentSuggestion]);

      setOCRResults((previous) => ({
        ...previous,
        [result.documentId]: {
          ...previous[result.documentId],
          saved: true,
        },
      }));
      setDocuments((previous) =>
        previous.map((document) =>
          document.id === result.documentId
            ? { ...document, content: result.combinedText }
            : document
        )
      );
    } catch (err) {
      console.error("Error saving OCR content:", err);
      setError(`Failed to save OCR content for document ${result.documentId}.`);
    } finally {
      setSavingOCRDocumentId(null);
    }
  };

  const handleSaveAllOCRContent = async () => {
    for (const documentId of selectedDocuments) {
      const result = ocrResults[documentId];
      if (result?.combinedText && !result.saved) {
        await handleSaveOCRContent(result);
      }
    }
  };

  const handleContinueAfterOCR = async (skipSaving = false) => {
    if (skipSaving) {
      setOCRSkipAccepted(true);
    }
    if (!workflowModeUsesSuggestions) {
      setActiveStep("select");
      return;
    }
    if (!skipSaving && !allSelectedOCRReadyForSuggestions) {
      setError("Save OCR content first, or explicitly continue without saving OCR text.");
      return;
    }
    await startSuggestions();
  };

  const handleStopSuggestionJob = async () => {
    if (!suggestionJobId) return;

    try {
      await axios.post(`./api/jobs/suggestions/${suggestionJobId}/stop`);
      setSuggestionJobStatus("cancelled");
      setError("Suggestion job cancelled.");
      clearActiveSuggestionJob();
    } catch (err) {
      console.error("Error stopping suggestion job:", err);
      setError("Failed to stop suggestion job.");
    }
  };

  const handleUpdateDocuments = async () => {
    setUpdating(true);
    setError(null);
    try {
      const payload = suggestions.map((suggestion) => {
        const { suggested_custom_fields, ...rest } = suggestion;
        const selectedCustomFields = suggested_custom_fields?.filter((cf) => cf.isSelected);
        return {
          ...rest,
          suggested_custom_fields: selectedCustomFields,
          custom_fields_write_mode: selectedCustomFields && selectedCustomFields.length > 0 ? "update" : rest.custom_fields_write_mode,
        };
      });

      await axios.patch("./api/update-documents", payload);
      setIsSuccessModalOpen(true);
      setSuggestions([]);
      setActiveStep("select");
    } catch (err) {
      console.error("Error updating documents:", err);
      if (axios.isAxiosError(err) && typeof err.response?.data?.error === "string") {
        setError(err.response.data.error);
      } else {
        setError("Failed to update documents.");
      }
    } finally {
      setUpdating(false);
    }
  };

  const handleTagAddition = (docId: number, tag: TagOption) => {
    setSuggestions((prevSuggestions) =>
      prevSuggestions.map((doc) => {
        if (doc.id !== docId) return doc;

        const originalTags = doc.original_document.tags || [];
        const tagName = tag.name;
        const removeTags = uniqueTags(doc.remove_tags || []).filter((removedTag) => !tagEquals(removedTag, tagName));
        const addTags = includesTag(originalTags, tagName)
          ? uniqueTags(doc.add_tags || [])
          : uniqueTags([...(doc.add_tags || []), tagName]);

        return {
          ...doc,
          keep_original_tags: true,
          add_tags: addTags,
          remove_tags: removeTags,
          suggested_tags: buildSelectedTags(originalTags, addTags, removeTags),
        };
      })
    );
  };

  const handleCustomFieldSuggestionToggle = (docId: number, fieldId: number) => {
    setSuggestions((prevSuggestions) =>
      prevSuggestions.map((doc) =>
        doc.id === docId
          ? {
              ...doc,
              suggested_custom_fields: doc.suggested_custom_fields?.map((cf) =>
                cf.id === fieldId ? { ...cf, isSelected: !cf.isSelected } : cf
              ),
            }
          : doc
      )
    );
  };

  const handleCustomFieldSuggestionValueChange = (docId: number, fieldId: number, value: string) => {
    setSuggestions((prevSuggestions) =>
      prevSuggestions.map((doc) =>
        doc.id === docId
          ? {
              ...doc,
              suggested_custom_fields: doc.suggested_custom_fields?.map((cf) =>
                cf.id === fieldId ? { ...cf, value, isSelected: true } : cf
              ),
            }
          : doc
      )
    );
  };

  const handleTagDeletion = (docId: number, tagName: string) => {
    setSuggestions((prevSuggestions) =>
      prevSuggestions.map((doc) => {
        if (doc.id !== docId) return doc;

        const originalTags = doc.original_document.tags || [];
        const addTags = uniqueTags(doc.add_tags || []).filter((addedTag) => !tagEquals(addedTag, tagName));
        const removeTags = includesTag(originalTags, tagName)
          ? uniqueTags([...(doc.remove_tags || []), tagName])
          : uniqueTags(doc.remove_tags || []);

        return {
          ...doc,
          keep_original_tags: true,
          add_tags: addTags,
          remove_tags: removeTags,
          suggested_tags: buildSelectedTags(originalTags, addTags, removeTags),
        };
      })
    );
  };

  const handleTagRestore = (docId: number, tagName: string) => {
    setSuggestions((prevSuggestions) =>
      prevSuggestions.map((doc) => {
        if (doc.id !== docId) return doc;

        const originalTags = doc.original_document.tags || [];
        const addTags = uniqueTags(doc.add_tags || []);
        const removeTags = uniqueTags(doc.remove_tags || []).filter((removedTag) => !tagEquals(removedTag, tagName));

        return {
          ...doc,
          keep_original_tags: true,
          add_tags: addTags,
          remove_tags: removeTags,
          suggested_tags: buildSelectedTags(originalTags, addTags, removeTags),
        };
      })
    );
  };

  const handleTitleChange = (docId: number, title: string) => {
    setSuggestions((prevSuggestions) =>
      prevSuggestions.map((doc) => (doc.id === docId ? { ...doc, suggested_title: title } : doc))
    );
  };

  const handleCorrespondentChange = (docId: number, correspondent: string) => {
    setSuggestions((prevSuggestions) =>
      prevSuggestions.map((doc) => (doc.id === docId ? { ...doc, suggested_correspondent: correspondent } : doc))
    );
  };

  const handleDocumentTypeChange = (docId: number, documentType: string) => {
    setSuggestions((prevSuggestions) =>
      prevSuggestions.map((doc) => (doc.id === docId ? { ...doc, suggested_document_type: documentType } : doc))
    );
  };

  const handleCreatedDateChange = (docId: number, createdDate: string) => {
    setSuggestions((prevSuggestions) =>
      prevSuggestions.map((doc) => (doc.id === docId ? { ...doc, suggested_created_date: createdDate } : doc))
    );
  };

  const resetSuggestions = () => {
    setSuggestions([]);
    setActiveStep("select");
  };

  const workflowStepItems = [
    { id: "select", label: "Select" },
    ...(workflowModeUsesOCR ? [{ id: "ocr", label: "OCR" }] : []),
    ...(workflowModeUsesSuggestions ? [{ id: "suggestions", label: "Suggestions" }] : []),
    ...(workflowModeUsesSuggestions ? [{ id: "review", label: "Review" }] : []),
  ];

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-white dark:bg-gray-900">
        <div className="text-xl font-semibold text-gray-800 dark:text-gray-200">
          Loading documents...
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-6xl bg-white p-6 text-gray-800 dark:bg-gray-900 dark:text-gray-200">
      <header className="mb-6">
        <h1 className="text-3xl font-bold text-gray-900 dark:text-gray-100">Paperless GPT</h1>
        <p className="mt-2 max-w-3xl text-sm text-gray-600 dark:text-gray-300">
          Select documents, choose the processing path, review generated text and metadata, then apply only the changes you approve.
        </p>
      </header>

      <nav aria-label="Workflow progress" className="mb-6">
        <ol className="grid gap-2 sm:grid-cols-4">
          {workflowStepItems.map((step, index) => {
            const isCurrent = activeStep === step.id;
            return (
              <li
                key={step.id}
                aria-current={isCurrent ? "step" : undefined}
                className={`rounded-md border px-3 py-2 text-sm font-medium ${
                  isCurrent
                    ? "border-blue-500 bg-blue-50 text-blue-800 dark:border-blue-400 dark:bg-blue-950 dark:text-blue-100"
                    : "border-gray-200 bg-white text-gray-600 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300"
                }`}
              >
                {index + 1}. {step.label}
              </li>
            );
          })}
        </ol>
      </nav>

      {error && (
        <div className="mb-4 rounded-md bg-red-100 p-4 text-red-800 dark:bg-red-900 dark:text-red-200" role="alert">
          {error}
        </div>
      )}

      <div aria-live="polite" className="sr-only">
        Active workflow step: {activeStep}. {processing ? "A job is running." : "No job is running."}
      </div>

      {documents.length === 0 ? (
        <NoDocuments filterTag={filterTag} onReload={reloadDocuments} processing={processing} />
      ) : suggestions.length > 0 ? (
        <SuggestionsReview
          suggestions={suggestions}
          availableTags={availableTags}
          availableDocumentTypes={availableDocumentTypes}
          createNewDocumentTypesEnabled={createNewDocumentTypesEnabled}
          onTitleChange={handleTitleChange}
          onTagAddition={handleTagAddition}
          onTagDeletion={handleTagDeletion}
          onTagRestore={handleTagRestore}
          onCorrespondentChange={handleCorrespondentChange}
          onDocumentTypeChange={handleDocumentTypeChange}
          onCreatedDateChange={handleCreatedDateChange}
          onCustomFieldSuggestionToggle={handleCustomFieldSuggestionToggle}
          onCustomFieldSuggestionValueChange={handleCustomFieldSuggestionValueChange}
          onBack={resetSuggestions}
          onUpdate={handleUpdateDocuments}
          updating={updating}
        />
      ) : (
        <>
          <section className="mb-6 rounded-md border border-gray-200 bg-white p-4 shadow-sm dark:border-gray-700 dark:bg-gray-800">
            <RadioGroup value={workflowMode} onChange={setWorkflowMode}>
              <RadioGroup.Label className="text-lg font-semibold text-gray-900 dark:text-gray-100">
                Workflow mode
              </RadioGroup.Label>
              <div className="mt-4 grid gap-3 md:grid-cols-3">
                {workflowModes
                  .filter((mode) => !mode.requiresOCR || ocrEnabled)
                  .map((mode) => (
                    <RadioGroup.Option
                      key={mode.id}
                      value={mode.id}
                      disabled={processing}
                      className={({ checked, disabled }) =>
                        `cursor-pointer rounded-md border p-4 focus:outline-none focus:ring-2 focus:ring-blue-500 ${
                          checked
                            ? "border-blue-500 bg-blue-50 dark:border-blue-400 dark:bg-blue-950"
                            : "border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-900"
                        } ${disabled ? "cursor-not-allowed opacity-60" : ""}`
                      }
                    >
                      {({ checked }) => (
                        <div className="flex gap-3">
                          <span
                            aria-hidden="true"
                            className={`mt-0.5 h-5 w-5 shrink-0 rounded-full border ${checked ? "border-blue-600 bg-blue-600" : "border-gray-400"}`}
                          />
                          <div>
                            <RadioGroup.Label as="p" className="font-medium text-gray-900 dark:text-gray-100">
                              {mode.title}
                            </RadioGroup.Label>
                            <RadioGroup.Description as="p" className="mt-1 text-sm text-gray-600 dark:text-gray-300">
                              {mode.description}
                            </RadioGroup.Description>
                          </div>
                        </div>
                      )}
                    </RadioGroup.Option>
                  ))}
              </div>
            </RadioGroup>
          </section>

          {workflowModeUsesSuggestions && (
            <section className="mb-6 rounded-md border border-gray-200 bg-white p-4 shadow-sm dark:border-gray-700 dark:bg-gray-800">
              <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
                <div>
                  <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Suggestion fields</h2>
                  <p className="mt-1 text-sm text-gray-600 dark:text-gray-300">
                    Choose a preset or adjust individual fields before generating metadata suggestions.
                  </p>
                </div>
                <RadioGroup value={suggestionPreset} onChange={applySuggestionPreset} className="w-full lg:max-w-3xl">
                  <RadioGroup.Label className="sr-only">Suggestion preset</RadioGroup.Label>
                  <div className="grid gap-2 md:grid-cols-4">
                    {suggestionPresetOptions.map((preset) => (
                      <RadioGroup.Option
                        key={preset.id}
                        value={preset.id}
                        disabled={processing}
                        className={({ checked }) =>
                          `cursor-pointer rounded-md border p-3 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 ${
                            checked
                              ? "border-blue-500 bg-blue-50 text-blue-900 dark:border-blue-400 dark:bg-blue-950 dark:text-blue-100"
                              : "border-gray-200 bg-white text-gray-700 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-200"
                          }`
                        }
                      >
                        <RadioGroup.Label className="font-medium">{preset.title}</RadioGroup.Label>
                        <RadioGroup.Description className="mt-1 text-xs text-gray-600 dark:text-gray-300">
                          {preset.description}
                        </RadioGroup.Description>
                      </RadioGroup.Option>
                    ))}
                  </div>
                </RadioGroup>
              </div>

              <fieldset className="mt-4">
                <legend className="text-sm font-medium text-gray-800 dark:text-gray-200">Advanced field toggles</legend>
                <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  {[
                    { label: "Generate titles", checked: generateTitles, onChange: markCustomPreset(setGenerateTitles) },
                    { label: "Generate tags", checked: generateTags, onChange: markCustomPreset(setGenerateTags) },
                    { label: "Generate correspondents", checked: generateCorrespondents, onChange: markCustomPreset(setGenerateCorrespondents) },
                    { label: "Generate document types", checked: generateDocumentTypes, onChange: markCustomPreset(setGenerateDocumentTypes) },
                    { label: "Generate created dates", checked: generateCreatedDate, onChange: markCustomPreset(setGenerateCreatedDate) },
                    { label: "Generate custom fields", checked: generateCustomFields, onChange: markCustomPreset(setGenerateCustomFields) },
                  ].map((field) => (
                    <label key={field.label} className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                      <input
                        type="checkbox"
                        checked={field.checked}
                        disabled={processing}
                        onChange={(event) => field.onChange(event.target.checked)}
                        className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                      />
                      {field.label}
                    </label>
                  ))}
                </div>
              </fieldset>
            </section>
          )}

          <DocumentsToProcess
            documents={documents}
            selectedDocuments={selectedDocuments}
            onSelectDocument={handleSelectDocument}
            onSelectAll={() => setSelectedDocuments(documents.map((doc) => doc.id))}
            onSelectNone={() => setSelectedDocuments([])}
            onReload={reloadDocuments}
            disabled={processing}
            title="Document queue"
            filterTag={filterTag}
          />

          <div className="sticky bottom-0 mt-6 border-t border-gray-200 bg-white/95 py-4 backdrop-blur dark:border-gray-700 dark:bg-gray-900/95">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <p className="text-sm text-gray-600 dark:text-gray-300">
                {selectedDocuments.length} selected. Current mode:{" "}
                <span className="font-medium">
                  {workflowModes.find((mode) => mode.id === workflowMode)?.title}
                </span>
              </p>
              <div className="flex flex-wrap gap-2">
                {processing && ocrJobId && activeStep === "ocr" && (
                  <button
                    type="button"
                    onClick={handleStopOCRJob}
                    className="inline-flex items-center gap-2 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700"
                  >
                    <StopIcon aria-hidden="true" className="h-4 w-4" />
                    Stop OCR
                  </button>
                )}
                {processing && suggestionJobId && activeStep === "suggestions" && (
                  <button
                    type="button"
                    onClick={handleStopSuggestionJob}
                    className="inline-flex items-center gap-2 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700"
                  >
                    <StopIcon aria-hidden="true" className="h-4 w-4" />
                    Stop suggestions
                  </button>
                )}
                <button
                  type="button"
                  onClick={startWorkflow}
                  disabled={
                    processing ||
                    selectedDocuments.length === 0 ||
                    (workflowModeUsesSuggestions && !(generateTitles || generateTags || generateCorrespondents || generateDocumentTypes || generateCreatedDate || generateCustomFields))
                  }
                  className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-800 dark:hover:bg-blue-900"
                >
                  <ArrowPathIcon aria-hidden="true" className="h-4 w-4" />
                  {processing ? "Processing..." : workflowModeUsesOCR ? "Start OCR workflow" : "Generate suggestions"}
                </button>
              </div>
            </div>
          </div>

          {activeStep === "ocr" && (
            <section className="mt-6 rounded-md border border-gray-200 bg-white p-4 shadow-sm dark:border-gray-700 dark:bg-gray-800">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <h2 className="text-xl font-semibold text-gray-900 dark:text-gray-100">OCR review</h2>
                  <p className="mt-1 text-sm text-gray-600 dark:text-gray-300">
                    Review OCR output before saving it back to Paperless or continuing to metadata suggestions.
                  </p>
                </div>
                <div className="text-sm text-gray-600 dark:text-gray-300">
                  {currentOCRDocumentId ? `Current document ${currentOCRDocumentId}` : "No OCR job running"}
                </div>
              </div>

              {processing && currentOCRDocumentId && (
                <div className="mt-4 rounded-md bg-blue-50 p-3 text-blue-800 dark:bg-blue-950 dark:text-blue-100">
                  <div className="flex items-center justify-between gap-4">
                    <span>OCR job {ocrJobStatus}</span>
                    {ocrTotalPages ? <span>{ocrPagesDone} / {ocrTotalPages} pages</span> : null}
                  </div>
                  {ocrTotalPages ? (
                    <progress
                      aria-label="OCR page progress"
                      className="mt-2 h-2 w-full"
                      max={ocrTotalPages}
                      value={ocrPagesDone}
                    />
                  ) : null}
                </div>
              )}

              <div className="mt-4 space-y-4">
                {selectedDocuments.map((documentId) => {
                  const result = ocrResults[documentId];
                  const document = result?.document || documents.find((doc) => doc.id === documentId);
                  if (!document) return null;

                  return (
                    <div key={documentId} className="rounded-md border border-gray-200 p-4 dark:border-gray-700">
                      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                        <div>
                          <h3 className="font-semibold text-gray-900 dark:text-gray-100">
                            {document.title || `Document ${documentId}`}
                          </h3>
                          <p className="text-sm text-gray-600 dark:text-gray-300">Document ID {documentId}</p>
                        </div>
                        {result?.saved && (
                          <span className="inline-flex items-center gap-1 rounded-full bg-green-100 px-2.5 py-1 text-xs font-medium text-green-800 dark:bg-green-900 dark:text-green-100">
                            <CheckCircleIcon aria-hidden="true" className="h-4 w-4" />
                            OCR saved
                          </span>
                        )}
                      </div>

                      {result?.error ? (
                        <div className="mt-3 rounded-md bg-red-50 p-3 text-sm text-red-800 dark:bg-red-950 dark:text-red-100">
                          {result.error}
                        </div>
                      ) : result?.combinedText ? (
                        <>
                          <div className="mt-4 grid gap-4 lg:grid-cols-2">
                            <div>
                              <h4 className="text-sm font-medium text-gray-700 dark:text-gray-200">Current Paperless text</h4>
                              <pre className="mt-2 max-h-80 overflow-auto rounded-md border border-gray-200 bg-gray-50 p-3 whitespace-pre-wrap text-sm dark:border-gray-700 dark:bg-gray-900">
                                {document.content || "No extracted document text is available yet."}
                              </pre>
                            </div>
                            <div>
                              <h4 className="text-sm font-medium text-gray-700 dark:text-gray-200">OCR result</h4>
                              <pre className="mt-2 max-h-80 overflow-auto rounded-md border border-gray-200 bg-gray-50 p-3 whitespace-pre-wrap text-sm dark:border-gray-700 dark:bg-gray-900">
                                {result.combinedText}
                              </pre>
                            </div>
                          </div>

                          {result.perPageResults.length > 0 && (
                            <div className="mt-4 space-y-2">
                              {result.perPageResults.map((page, index) => (
                                <Disclosure key={index}>
                                  {({ open }) => (
                                    <div className="rounded-md border border-gray-200 dark:border-gray-700">
                                      <DisclosureButton className="flex w-full items-center justify-between px-3 py-2 text-left text-sm font-medium text-gray-800 dark:text-gray-100">
                                        <span>
                                          Page {index + 1}
                                          {page.ocrLimitHit ? (
                                            <span className="ml-2 rounded-full bg-yellow-100 px-2 py-0.5 text-xs text-yellow-800 dark:bg-yellow-900 dark:text-yellow-100">
                                              Token limit hit
                                            </span>
                                          ) : null}
                                        </span>
                                        <ChevronDownIcon aria-hidden="true" className={`h-4 w-4 transition ${open ? "rotate-180" : ""}`} />
                                      </DisclosureButton>
                                      <DisclosurePanel className="border-t border-gray-200 p-3 dark:border-gray-700">
                                        {page.generationInfo && Object.keys(page.generationInfo).length > 0 && (
                                          <dl className="mb-3 grid gap-2 text-xs text-gray-600 dark:text-gray-300 sm:grid-cols-2">
                                            {Object.entries(page.generationInfo).map(([key, value]) => (
                                              <div key={key}>
                                                <dt className="font-medium">{key}</dt>
                                                <dd className="break-all">{formatGenerationInfo(value)}</dd>
                                              </div>
                                            ))}
                                          </dl>
                                        )}
                                        <pre className="max-h-64 overflow-auto whitespace-pre-wrap rounded bg-gray-50 p-3 text-sm dark:bg-gray-900">
                                          {page.text}
                                        </pre>
                                      </DisclosurePanel>
                                    </div>
                                  )}
                                </Disclosure>
                              ))}
                            </div>
                          )}

                          {selectedDocuments.length > 1 && (
                            <div className="mt-4 flex flex-wrap gap-2">
                              <button
                                type="button"
                                onClick={() => handleSaveOCRContent(result)}
                                disabled={savingOCRDocumentId === result.documentId || result.saved}
                                className="rounded-md bg-green-600 px-4 py-2 text-sm font-medium text-white hover:bg-green-700 disabled:cursor-not-allowed disabled:opacity-50"
                              >
                                {savingOCRDocumentId === result.documentId ? "Saving..." : result.saved ? "OCR saved" : "Save OCR content"}
                              </button>
                            </div>
                          )}
                        </>
                      ) : (
                        <p className="mt-3 text-sm text-gray-600 dark:text-gray-300">Waiting for OCR result.</p>
                      )}
                    </div>
                  );
                })}
              </div>

              {allSelectedOCRComplete && (
                <div className="mt-4 flex flex-wrap justify-end gap-2 border-t border-gray-200 pt-4 dark:border-gray-700">
                  {unsavedOCRContentCount > 0 && (
                    <button
                      type="button"
                      onClick={handleSaveAllOCRContent}
                      disabled={savingOCRDocumentId !== null || !hasUnsavedOCRContent}
                      className="rounded-md bg-green-600 px-4 py-2 text-sm font-medium text-white hover:bg-green-700 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      {selectedDocuments.length === 1 ? "Save OCR content" : "Save all unsaved OCR content"}
                    </button>
                  )}
                  {workflowModeUsesSuggestions && (
                    <>
                      <button
                        type="button"
                        onClick={() => handleContinueAfterOCR(true)}
                        className="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-700"
                      >
                        Continue without saving OCR
                      </button>
                      <button
                        type="button"
                        onClick={() => handleContinueAfterOCR(false)}
                        disabled={!allSelectedOCRReadyForSuggestions}
                        className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
                      >
                        Continue to suggestions
                      </button>
                    </>
                  )}
                </div>
              )}
            </section>
          )}

          {activeStep === "suggestions" && (
            <section className="mt-6 rounded-md border border-gray-200 bg-blue-50 p-4 text-blue-900 shadow-sm dark:border-blue-900 dark:bg-blue-950 dark:text-blue-100">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <h2 className="text-lg font-semibold">Generating suggestions</h2>
                  <p className="mt-1 text-sm">
                    Suggestion job {suggestionJobStatus}: {documentsDone} / {totalDocuments} documents processed
                    {currentDocumentId ? `, current document ${currentDocumentId}` : ""}
                  </p>
                </div>
                {totalDocuments > 0 && (
                  <progress
                    aria-label="Suggestion job document progress"
                    className="h-2 w-full sm:max-w-xs"
                    max={totalDocuments}
                    value={documentsDone}
                  />
                )}
              </div>
            </section>
          )}
        </>
      )}

      <SuccessModal
        isOpen={isSuccessModalOpen}
        onClose={() => {
          setIsSuccessModalOpen(false);
          reloadDocuments();
        }}
      />
    </div>
  );
};

export default DocumentProcessor;
