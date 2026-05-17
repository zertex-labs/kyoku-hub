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
