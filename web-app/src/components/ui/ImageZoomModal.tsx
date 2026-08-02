import {
  ArrowsPointingOutIcon,
  MagnifyingGlassMinusIcon,
  MagnifyingGlassPlusIcon,
} from "@heroicons/react/24/outline";
import React, { useState } from "react";
import Button from "./Button";
import Modal from "./Modal";

interface ImageZoomModalProps {
  open: boolean;
  onClose: () => void;
  src: string;
  alt: string;
  title: string;
}

const ZOOM_LEVELS = [0.5, 0.75, 1, 1.5, 2, 3, 4];
const DEFAULT_ZOOM_INDEX = 2; // 100%

/** Full-size scan preview with zoom controls, opened from any small thumbnail. */
const ImageZoomModal: React.FC<ImageZoomModalProps> = ({
  open,
  onClose,
  src,
  alt,
  title,
}) => {
  // Callers key this component by `src` so a new image always remounts with
  // a fresh 100% zoom instead of carrying over the previous image's level.
  const [zoomIndex, setZoomIndex] = useState(DEFAULT_ZOOM_INDEX);
  const zoom = ZOOM_LEVELS[zoomIndex];

  return (
    <Modal open={open} onClose={onClose} size="focus" hideTitle title={title}>
      <div className="flex shrink-0 items-center justify-between gap-3 border-b border-line px-4 py-2">
        <p className="min-w-0 truncate text-sm font-medium" title={title}>
          {title}
        </p>
        <div className="flex shrink-0 items-center gap-1">
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setZoomIndex((i) => Math.max(0, i - 1))}
            disabled={zoomIndex === 0}
            aria-label="Zoom out"
          >
            <MagnifyingGlassMinusIcon className="h-4 w-4" aria-hidden="true" />
          </Button>
          <span className="w-12 text-center text-xs tabular-nums text-muted">
            {Math.round(zoom * 100)}%
          </span>
          <Button
            size="sm"
            variant="ghost"
            onClick={() =>
              setZoomIndex((i) => Math.min(ZOOM_LEVELS.length - 1, i + 1))
            }
            disabled={zoomIndex === ZOOM_LEVELS.length - 1}
            aria-label="Zoom in"
          >
            <MagnifyingGlassPlusIcon className="h-4 w-4" aria-hidden="true" />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setZoomIndex(DEFAULT_ZOOM_INDEX)}
            disabled={zoomIndex === DEFAULT_ZOOM_INDEX}
            aria-label="Reset zoom to 100%"
          >
            <ArrowsPointingOutIcon className="h-4 w-4" aria-hidden="true" />
          </Button>
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-auto bg-surface-2 p-4">
        <img
          src={src}
          alt={alt}
          style={{ width: `${zoom * 100}%`, maxWidth: "none" }}
          className="rounded border border-line bg-surface shadow-card"
        />
      </div>
    </Modal>
  );
};

export default ImageZoomModal;
