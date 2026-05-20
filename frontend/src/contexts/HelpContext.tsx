import { createContext, useContext, useState, useCallback, ReactNode } from "react";
import HelpModal from "../components/HelpModal";
import { helpContent } from "../help-content";

interface HelpContextValue {
  showHelp: (key: string) => void;
}

const HelpContext = createContext<HelpContextValue | null>(null);

export function HelpProvider({ children }: { children: ReactNode }) {
  const [helpKey, setHelpKey] = useState<string | null>(null);

  const showHelp = useCallback((key: string) => {
    setHelpKey(key);
  }, []);

  const closeHelp = useCallback(() => {
    setHelpKey(null);
  }, []);

  return (
    <HelpContext.Provider value={{ showHelp }}>
      {children}
      {helpKey && (
        <HelpModalWrapper key={helpKey} helpKey={helpKey} onClose={closeHelp} />
      )}
    </HelpContext.Provider>
  );
}

function HelpModalWrapper({ helpKey, onClose }: { helpKey: string; onClose: () => void }) {
  const entry = helpContent[helpKey];
  if (!entry) {
    onClose();
    return null;
  }
  return <HelpModal title={entry.title} content={entry.content} onClose={onClose} />;
}

export function useHelp(): HelpContextValue {
  const ctx = useContext(HelpContext);
  if (!ctx) throw new Error("useHelp must be used within HelpProvider");
  return ctx;
}
