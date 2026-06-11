export type TrackIssue = {
  rpath: string;
  missing: string[];
};

export type UntaggedTrack = {
  agent_id: string;
  rpath: string;
  title: string;
  album: string;
  artist: string;
};

export type FsSyncedEvent = {
  agent_id: string;
  tracks: number;
  release_groups: number;
  untagged: number;
};
