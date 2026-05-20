import { useEffect, useRef } from "react";
import { X } from "lucide-react";

interface HelpModalProps {
  title: string;
  content: string;
  onClose: () => void;
}

export default function HelpModal({ title, content, onClose }: HelpModalProps) {
  const overlayRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [onClose]);

  return (
    <div
      ref={overlayRef}
      className="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
      onClick={(e) => {
        if (e.target === overlayRef.current) onClose();
      }}
    >
      <div className="bg-panel border border-panel2 rounded-xl shadow-2xl max-w-lg w-full max-h-[80vh] flex flex-col">
        <div className="flex items-center justify-between px-5 py-4 border-b border-panel2 flex-shrink-0">
          <h2 className="text-lg font-semibold text-text">{title}</h2>
          <button
            onClick={onClose}
            className="p-1 text-muted hover:text-text transition-colors rounded-md hover:bg-panel2"
            aria-label="Close help"
          >
            <X size={18} />
          </button>
        </div>
        <div className="px-5 py-4 overflow-y-auto flex-1">
          <div className="text-sm text-text leading-relaxed whitespace-pre-wrap">
            {content}
          </div>
        </div>
        <div className="px-5 py-3 border-t border-panel2 flex-shrink-0">
          <button
            onClick={onClose}
            className="w-full px-4 py-2 bg-panel2 hover:bg-panel2/70 text-text rounded-md text-sm transition-colors"
          >
            Got it
          </button>
        </div>
      </div>
    </div>
  );
}

/** Small help button that opens a modal */
export function HelpButton({
  onClick,
  className = "",
}: {
  onClick: () => void;
  className?: string;
}) {
  return (
    <button
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
      className={`inline-flex items-center justify-center w-5 h-5 rounded-full bg-panel2/60 hover:bg-accent/30 text-muted hover:text-accent text-[10px] font-bold transition-colors flex-shrink-0 ${className}`}
      aria-label="Help"
      title="Click for help"
    >
      ?
    </button>
  );
}
