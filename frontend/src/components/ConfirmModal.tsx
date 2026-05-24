import { useEffect, useRef, useCallback } from "react";
import { X, AlertTriangle } from "lucide-react";

interface ConfirmModalProps {
  open: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: "danger" | "warning" | "info";
  onConfirm: () => void;
  onCancel: () => void;
}

export default function ConfirmModal({
  open,
  title,
  message,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  variant = "danger",
  onConfirm,
  onCancel,
}: ConfirmModalProps) {
  const overlayRef = useRef<HTMLDivElement>(null);
  const confirmRef = useRef<HTMLButtonElement>(null);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === "Escape") onCancel();
    },
    [onCancel]
  );

  useEffect(() => {
    if (open) {
      document.addEventListener("keydown", handleKeyDown);
      // Focus the cancel button so Enter doesn't immediately confirm
      setTimeout(() => confirmRef.current?.focus(), 0);
    }
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [open, handleKeyDown]);

  if (!open) return null;

  const confirmClass =
    variant === "danger"
      ? "bg-red-500 hover:bg-red-600 text-white"
      : variant === "warning"
        ? "bg-yellow-500 hover:bg-yellow-600 text-black"
        : "bg-accent hover:bg-accent/80 text-white";

  return (
    <>
      <div
        ref={overlayRef}
        className="fixed inset-0 z-[100] bg-black/60"
        onClick={onCancel}
      />
      <div
        className="fixed inset-0 z-[100] flex items-center justify-center p-4"
        role="alertdialog"
        aria-modal="true"
        aria-label={title}
      >
        <div className="bg-panel border border-panel2 rounded-xl p-6 max-w-sm w-full shadow-xl">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-2">
              {variant === "danger" && (
                <AlertTriangle size={18} className="text-red-400" />
              )}
              {variant === "warning" && (
                <AlertTriangle size={18} className="text-yellow-400" />
              )}
              <h3 className="text-sm font-medium text-text">{title}</h3>
            </div>
            <button
              onClick={onCancel}
              className="p-1 text-muted hover:text-text"
              aria-label="Close"
            >
              <X size={16} />
            </button>
          </div>
          <p className="text-sm text-muted mb-6">{message}</p>
          <div className="flex gap-3 justify-end">
            <button
              onClick={onCancel}
              className="px-4 py-2 rounded-md text-sm text-muted hover:text-text border border-panel2 hover:border-panel2/80 transition-colors"
            >
              {cancelLabel}
            </button>
            <button
              ref={confirmRef}
              onClick={onConfirm}
              className={`px-4 py-2 rounded-md text-sm font-medium transition-colors ${confirmClass}`}
            >
              {confirmLabel}
            </button>
          </div>
        </div>
      </div>
    </>
  );
}
