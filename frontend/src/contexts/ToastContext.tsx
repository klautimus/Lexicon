import {
  createContext,
  useContext,
  useState,
  useCallback,
  ReactNode,
} from "react";
import { X, CheckCircle, AlertCircle, Info } from "lucide-react";

interface Toast {
  id: string;
  message: string;
  type: "success" | "error" | "info";
}

interface ToastContextValue {
  success: (message: string) => void;
  error: (message: string) => void;
  info: (message: string) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const remove = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const add = useCallback(
    (message: string, type: Toast["type"]) => {
      const id = Math.random().toString(36).slice(2);
      setToasts((prev) => [...prev, { id, message, type }]);
      setTimeout(() => remove(id), 4500);
    },
    [remove]
  );

  const success = useCallback(
    (message: string) => add(message, "success"),
    [add]
  );
  const error = useCallback(
    (message: string) => add(message, "error"),
    [add]
  );
  const info = useCallback(
    (message: string) => add(message, "info"),
    [add]
  );

  const iconMap = {
    success: <CheckCircle size={16} className="text-green-400 shrink-0" />,
    error: <AlertCircle size={16} className="text-red-400 shrink-0" />,
    info: <Info size={16} className="text-accent shrink-0" />,
  };

  const borderMap = {
    success: "border-l-green-500",
    error: "border-l-red-500",
    info: "border-l-accent",
  };

  return (
    <ToastContext.Provider value={{ success, error, info }}>
      {children}
      <div className="fixed top-4 right-4 z-50 space-y-2 max-w-sm">
        {toasts.map((t) => (
          <div
            key={t.id}
            className={`bg-panel2 border border-panel2 ${borderMap[t.type]} border-l-4 rounded-lg shadow-lg px-4 py-3 flex items-start gap-3 animate-[fadeIn_0.2s_ease-out]`}
          >
            {iconMap[t.type]}
            <p className="text-sm text-text flex-1">{t.message}</p>
            <button
              onClick={() => remove(t.id)}
              className="text-muted hover:text-text shrink-0 mt-0.5"
            >
              <X size={14} />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used within ToastProvider");
  return ctx;
}
