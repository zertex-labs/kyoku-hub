export interface FsAgent {
  id: string;
  label: string;
  created_at: string;
  last_seen_at?: string;
  connected: boolean;
}
