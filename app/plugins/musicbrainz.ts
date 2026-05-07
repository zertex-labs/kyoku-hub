import { MusicBrainzApi } from "musicbrainz-api";

export default defineNuxtPlugin((nuxtApp) => {
  const mbApi = new MusicBrainzApi({
    appName: "kyoku-hub",
    appVersion: "0.0.1-dev",
    appContactInfo: "contact@burningdown.house",
  });

  return {
    provide: {
      mbApi,
    },
  };
});
