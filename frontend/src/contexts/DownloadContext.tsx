import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  useRef,
  ReactNode,
} from "react";
import { api, DownloadJob, PlaylistPayload } from "../lib/api";
import { useToast } from "./ToastContext";

interface DownloadContextValue {
  // Download tracking
  downloadingIds: Set<string>;
  completedIds: Set<string>;
  completedTrackIds: Record<string, number>;

  // AI playlist state
  playlistPreview: PlaylistPayload | null;
  playlistTrackStatus: Record<
    string,
    "pending" | "present" | "downloading" | "completed" | "failed"
  >;
  createdPlaylistId: number | null;
  generatingPlaylist: boolean;
  creatingPlaylist: boolean;

  // Actions
  downloadItem: (title: string, artist: string) => Promise<void>;
  trackDownload: (job: DownloadJob, name: string) => void;
  generateAiPlaylist: (force?: boolean) => Promise<void>;
  createAiPlaylist: () => Promise<void>;
  clearPlaylistPreview: () => void;
  adoptPlaylistPreview: (playlist: PlaylistPayload) => void;
}

const DownloadContext = createContext<DownloadContextValue | null>(null);

export function DownloadProvider({ children }: { children: ReactNode }) {
  const toast = useToast();
  const pollRef = useRef<Record<string, number>>({});

  // Download tracking
  const [downloadingIds, setDownloadingIds] = useState<Set<string>>(new Set());
  const [completedIds, setCompletedIds] = useState<Set<string>>(new Set());
  const [completedTrackIds, setCompletedTrackIds] = useState<
    Record<string, number>
  >({});

  // AI playlist state
  const [playlistPreview, setPlaylistPreview] =
    useState<PlaylistPayload | null>(null);
  const [generatingPlaylist, setGeneratingPlaylist] = useState(false);
  const [creatingPlaylist, setCreatingPlaylist] = useState(false);
  const [playlistTrackStatus, setPlaylistTrackStatus] = useState<
    Record<
      string,
      "pending" | "present" | "downloading" | "completed" | "failed"
    >
  >({});
  const [createdPlaylistId, setCreatedPlaylistId] = useState<number | null>(
    null
  );

  // Cleanup intervals on unmount
  useEffect(() => {
    return () => {
      Object.values(pollRef.current).forEach(clearInterval);
    };
  }, []);

  const clearPlaylistPreview = useCallback(() => {
    setPlaylistPreview(null);
    setCreatedPlaylistId(null);
    setPlaylistTrackStatus({});
  }, []);

  const adoptPlaylistPreview = useCallback(
    (playlist: PlaylistPayload) => {
      setPlaylistPreview(playlist);
      setCreatedPlaylistId(null);
      const initStatus: Record<string, "pending"> = {};
      playlist.tracks.forEach((t) => {
        initStatus[`${t.artist} - ${t.title}`] = "pending";
      });
      setPlaylistTrackStatus(initStatus);
    },
    []
  );

  const trackDownload = useCallback(
    (job: DownloadJob, name: string) => {
      if (job.status === "succeeded") {
        toast.success(`"${name}" downloaded successfully!`);
        return;
      }
      if (job.status === "failed") {
        toast.error(`Download failed: ${job.error || "Unknown error"}`);
        return;
      }
      if (job.status === "cancelled") {
        toast.info(`Download cancelled for "${name}"`);
        return;
      }

      toast.info(`Download started for "${name}"`);
      const interval = window.setInterval(async () => {
        try {
          const updated = await api.downloadJob(job.id);
          if (updated.status === "succeeded") {
            window.clearInterval(interval);
            delete pollRef.current[job.id];
            toast.success(`"${name}" downloaded successfully!`);
            setDownloadingIds((prev) => {
              const next = new Set(prev);
              next.delete(name);
              return next;
            });
            setCompletedIds((prev) => new Set(prev).add(name));
            // Try to find the newly downloaded track so we can offer a Play button
            try {
              const tracks = await api.search(name);
              if (tracks.length > 0) {
                setCompletedTrackIds((prev) => ({
                  ...prev,
                  [name]: tracks[0].id,
                }));
              }
            } catch {
              /* ignore */
            }
          } else if (updated.status === "failed") {
            window.clearInterval(interval);
            delete pollRef.current[job.id];
            toast.error(
              `Download failed: ${updated.error || "Unknown error"}`
            );
            setDownloadingIds((prev) => {
              const next = new Set(prev);
              next.delete(name);
              return next;
            });
          } else if (updated.status === "cancelled") {
            window.clearInterval(interval);
            delete pollRef.current[job.id];
            toast.info(`Download cancelled for "${name}"`);
            setDownloadingIds((prev) => {
              const next = new Set(prev);
              next.delete(name);
              return next;
            });
          }
        } catch {
          window.clearInterval(interval);
          delete pollRef.current[job.id];
          toast.error(`Lost connection tracking download`);
          setDownloadingIds((prev) => {
            const next = new Set(prev);
            next.delete(name);
            return next;
          });
        }
      }, 2000);
      pollRef.current[job.id] = interval;
    },
    [toast]
  );

  const downloadItem = useCallback(
    async (title: string, artist: string) => {
      const name = `${artist} - ${title}`;
      if (downloadingIds.has(name)) return;
      setDownloadingIds((prev) => new Set(prev).add(name));
      try {
        const job = await api.downloadSearch(name);
        trackDownload(job, name);
      } catch {
        toast.error("Failed to start download");
        setDownloadingIds((prev) => {
          const next = new Set(prev);
          next.delete(name);
          return next;
        });
      }
    },
    [downloadingIds, toast, trackDownload]
  );

  const generateAiPlaylist = useCallback(async (force?: boolean) => {
    setGeneratingPlaylist(true);
    setPlaylistPreview(null);
    setCreatedPlaylistId(null);
    setPlaylistTrackStatus({});
    try {
      const data = await api.generatePlaylist(force);
      setPlaylistPreview(data);
      const initStatus: Record<string, "pending"> = {};
      data.tracks.forEach((t) => {
        initStatus[`${t.artist} - ${t.title}`] = "pending";
      });
      setPlaylistTrackStatus(initStatus);
    } catch (e: any) {
      toast.error("Failed to generate playlist: " + e.message);
    } finally {
      setGeneratingPlaylist(false);
    }
  }, [toast]);

  const createAiPlaylist = useCallback(async () => {
    if (!playlistPreview) return;
    setCreatingPlaylist(true);
    try {
      // 1. Create the playlist
      const playlist = await api.createPlaylist(playlistPreview.name);
      setCreatedPlaylistId(playlist.id);
      toast.success(`Created playlist "${playlistPreview.name}"`);

      // 2. Process each track via downloadSearch (backend checks library first)
      for (const track of playlistPreview.tracks) {
        const key = `${track.artist} - ${track.title}`;
        setPlaylistTrackStatus((prev) => ({ ...prev, [key]: "downloading" }));
        try {
          const job = await api.downloadSearch(key);

          // Case A: Backend resolved to existing track immediately
          if (job.status === "succeeded" && job.track_id) {
            await api.addToPlaylist(playlist.id, job.track_id);
            setPlaylistTrackStatus((prev) => ({ ...prev, [key]: "present" }));
            continue;
          }

          // Case B: Download in progress — poll until done
          const interval = window.setInterval(async () => {
            try {
              const updated = await api.downloadJob(job.id);
              if (updated.status === "succeeded") {
                window.clearInterval(interval);
                delete pollRef.current[job.id];

                // Sub-case B1: Backend populated track_id (resolved or after rescan)
                if (updated.track_id) {
                  await api.addToPlaylist(playlist.id, updated.track_id);
                  setPlaylistTrackStatus((prev) => ({
                    ...prev,
                    [key]: "completed",
                  }));
                  toast.success(`"${key}" added to playlist`);
                  return;
                }

                // Sub-case B2: Actual download completed — find track in library with retries
                let found = false;
                for (let attempt = 0; attempt < 15; attempt++) {
                  const searches = [
                    `${track.artist} ${track.title}`,
                    track.title,
                    track.artist,
                    key,
                  ];
                  for (const q of searches) {
                    try {
                      const tracks = await api.search(q);
                      if (tracks.length > 0) {
                        await api.addToPlaylist(
                          playlist.id,
                          tracks[0].id
                        );
                        setPlaylistTrackStatus((prev) => ({
                          ...prev,
                          [key]: "completed",
                        }));
                        toast.success(`"${key}" added to playlist`);
                        found = true;
                        break;
                      }
                    } catch {
                      /* continue */
                    }
                  }
                  if (found) break;
                  await new Promise((r) => setTimeout(r, 2000));
                }
                if (!found) {
                  setPlaylistTrackStatus((prev) => ({
                    ...prev,
                    [key]: "failed",
                  }));
                }
              } else if (
                updated.status === "failed" ||
                updated.status === "cancelled"
              ) {
                window.clearInterval(interval);
                delete pollRef.current[job.id];
                setPlaylistTrackStatus((prev) => ({
                  ...prev,
                  [key]: "failed",
                }));
              }
            } catch {
              window.clearInterval(interval);
              delete pollRef.current[job.id];
              setPlaylistTrackStatus((prev) => ({
                ...prev,
                [key]: "failed",
              }));
            }
          }, 2000);
          pollRef.current[job.id] = interval;
        } catch {
          setPlaylistTrackStatus((prev) => ({ ...prev, [key]: "failed" }));
        }
      }
    } catch (e: any) {
      toast.error("Failed to create playlist: " + e.message);
    } finally {
      setCreatingPlaylist(false);
    }
  }, [playlistPreview, toast]);

  return (
    <DownloadContext.Provider
      value={{
        downloadingIds,
        completedIds,
        completedTrackIds,
        playlistPreview,
        playlistTrackStatus,
        createdPlaylistId,
        generatingPlaylist,
        creatingPlaylist,
        downloadItem,
        trackDownload,
        generateAiPlaylist,
        createAiPlaylist,
        clearPlaylistPreview,
        adoptPlaylistPreview,
      }}
    >
      {children}
    </DownloadContext.Provider>
  );
}

export function useDownloads(): DownloadContextValue {
  const ctx = useContext(DownloadContext);
  if (!ctx) throw new Error("useDownloads must be used within DownloadProvider");
  return ctx;
}
