export type ArtistActivity = {
  name: string;
  image: string;
  type: "release" | "update" | "delete";
  content: string;
  ts: string;
};

export type IncompleteArtist = {
  name: string;
  image: string;
  have: number;
  total: number;
};

export interface TrackedArtist {
  mb_artist_id: string;
  name: string;
  disambiguation: string;
  image_url?: string;
  description: string;
  release_group_count: number;
  status: string;
  created_at: string;
  synced_at?: string;
}

export interface BrainzArtist {
  mb_artist_id: string;
  name: string;
  sort_name: string;
  disambiguation: string;
  type: string;
  country: string;
  image_url: string;
  description: string;
  last_fetched_at: string;
  created_at: string;
}

export interface ReleaseGroup {
  mb_release_group_id: string;
  title: string;
  primary_type: string;
  secondary_types: string;
  disambiguation: string;
  first_release_date: string;
}
