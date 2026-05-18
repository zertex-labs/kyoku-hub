import { MusicBrainzApi } from "musicbrainz-api";

export const mbApi = new MusicBrainzApi({
  appName: "kyoku-hub",
  appVersion: "0.0.1-dev",
  appContactInfo: "contact@burningdown.house",
  retryLimit: 3,
  retryDelay: 1000,
});
