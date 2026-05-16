import { useEffect, useState } from "react";
import { api, Track } from "../lib/api";
import TrackList from "../components/TrackList";

export default function PodcastsPage() {
  const [tracks, setTracks] = useState<Track[]>([]);
  useEffect(() => {
    api.tracks("podcast", 500).then(setTracks).catch(() => {});
  }, []);
  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold">Podcasts</h1>
      <TrackList tracks={tracks} onDelete={() => api.tracks("podcast", 500).then(setTracks)} />
    </div>
  );
}
