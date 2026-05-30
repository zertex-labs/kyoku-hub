package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zertex-labs/kyoku-hub/apps/fs/internal/config"

	tag "github.com/dhowden/tag"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
	"go.uber.org/zap"
)

type MusicBrainzTags struct {
	MusicBrainzTrackID        string `json:"musicbrainz_track_id"`
	MusicBrainzRecordingID    string `json:"musicbrainz_recording_id"`
	MusicBrainzReleaseID      string `json:"musicbrainz_release_id"`
	MusicBrainzReleaseGroupID string `json:"musicbrainz_release_group_id"`
	MusicBrainzArtistID       string `json:"musicbrainz_artist_id"`
	MusicBrainzAlbumArtistID  string `json:"musicbrainz_album_artist_id"`
}

type MusicTags struct {
	Format      tag.Format `json:"format"`
	Title       string     `json:"title"`
	Album       string     `json:"album"`
	Artist      string     `json:"artist"`
	AlbumArtist string     `json:"album_artist"`
	Composer    string     `json:"composer"`
	Genre       string     `json:"genre"`
	Year        int        `json:"year"`
	TrackNum    int        `json:"track_num"`
	TrackTotal  int        `json:"track_total"`
	DiscNum     int        `json:"disc_num"`
	DiscTotal   int        `json:"disc_total"`
	Comment     string     `json:"comment"`

	MusicBrainzTags
}

type BaseFileData struct {
	RelativePath string    `json:"rpath"`
	ModTime      time.Time `json:"mt"`
}

type FileData struct {
	BaseFileData
	MusicTags MusicTags `json:"mtags"`
}

type FileDataMap = map[string][]FileData

type FsHandler struct {
	logger *zap.Logger
	cfg    *config.Config
}

var SupportedFileExtensions = map[string]bool{
	".mp3":  true,
	".flac": true,
	".ogg":  true,
	".oga":  true,
	".m4a":  true,
	".m4b":  true,
	".m4p":  true,
	".mp4":  true,
	".alac": true,
}

func NewFsHandler(logger *zap.Logger, cfg *config.Config) *FsHandler {
	return &FsHandler{logger: logger, cfg: cfg}
}

func (h *FsHandler) RegisterRoutes(g *echo.Group) {
	g.GET("/data", h.GetData)
}

func (h *FsHandler) GetData(c echo.Context) error {
	dirs, err := h.ScanDirs(h.cfg.ScanDirs)

	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "failed to parse dirs"})
	}

	return c.JSON(http.StatusOK, dirs)
}

func (h *FsHandler) ScanDirs(dirs []string) (FileDataMap, error) {
	record := make(FileDataMap, len(dirs))
	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				h.logger.Error("got error from walk")
				return err
			}

			if info.IsDir() {
				return nil
			}

			ext := filepath.Ext(path)
			if !SupportedFileExtensions[strings.ToLower(ext)] {
				// TODO: error
				return nil
			}

			if record[dir] == nil {
				record[dir] = make([]FileData, 0)
			}
			rdata := record[dir]
			relativePath, err := filepath.Rel(dir, path)
			if err != nil {
				log.Error("failed to get relative path for " + path)
				return err
			}
			filedata, err := h.CompleteMetadata(dir, BaseFileData{
				ModTime:      info.ModTime(),
				RelativePath: relativePath,
			})
			if err != nil {
				log.Error("failed to complete metadata for " + path)
				return err
			}
			record[dir] = append(rdata, filedata)

			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	return record, nil
}

func (h *FsHandler) CompleteMetadata(dir string, base BaseFileData) (FileData, error) {
	filePath := strings.Join([]string{dir, base.RelativePath}, "/")
	h.logger.Info("completing path " + filePath)

	f, err := os.OpenFile(filePath, os.O_RDONLY, 0)
	if err != nil {
		return FileData{}, err
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return FileData{}, err
	}

	trackNum, trackTotal := m.Track()
	discNum, discTotal := m.Disc()
	format := m.Format()
	raw := m.Raw()
	return FileData{
		BaseFileData: base,
		MusicTags: MusicTags{
			Format:          format,
			Title:           m.Title(),
			Album:           m.Album(),
			Artist:          m.Artist(),
			AlbumArtist:     m.AlbumArtist(),
			Composer:        m.Composer(),
			Genre:           m.Genre(),
			Year:            m.Year(),
			TrackNum:        trackNum,
			TrackTotal:      trackTotal,
			DiscNum:         discNum,
			DiscTotal:       discTotal,
			Comment:         m.Comment(),
			MusicBrainzTags: getMusicBrainzTags(format, raw),
		},
	}, nil
}

func getRawTag(raw map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		// Try variations of the key to account for different taggers
		variations := []string{key, strings.ToLower(key), strings.ToUpper(key)}

		for _, vKey := range variations {
			if val, exists := raw[vKey]; exists {
				switch v := val.(type) {
				case string:
					return v
				case []byte:
					return string(v)
				case []string:
					if len(v) > 0 {
						return v[0]
					}
				}
			}
		}
	}
	return ""
}

func getMusicBrainzTags(format tag.Format, raw map[string]interface{}) MusicBrainzTags {
	var mbTrackID, mbRecordingID, mbReleaseID, mbReleaseGroupID, mbArtistID, mbAlbumArtistID string

	if format == tag.VORBIS {
		// Vorbis Comments (FLAC, OGG) — uppercase, underscore-separated
		mbTrackID = getRawTag(raw, "MUSICBRAINZ_RELEASETRACKID") 
		mbRecordingID = getRawTag(raw, "MUSICBRAINZ_TRACKID")   
		mbReleaseID = getRawTag(raw, "MUSICBRAINZ_ALBUMID")
		mbReleaseGroupID = getRawTag(raw, "MUSICBRAINZ_RELEASEGROUPID")
		mbArtistID = getRawTag(raw, "MUSICBRAINZ_ARTISTID")
		mbAlbumArtistID = getRawTag(raw, "MUSICBRAINZ_ALBUMARTISTID")
	} else {
		// ID3v2 (MP3) and MP4 — Picard writes these as TXXX frames
		// "MusicBrainz Track Id"         = the Recording ID (canonical audio fingerprint)
		// "MusicBrainz Release Track Id" = the Track ID on a specific release (positional)
		mbRecordingID = getRawTag(raw, "TXXX:MusicBrainz Track Id")
		mbTrackID = getRawTag(raw, "TXXX:MusicBrainz Release Track Id")
		mbReleaseID = getRawTag(raw, "TXXX:MusicBrainz Album Id")
		mbReleaseGroupID = getRawTag(raw, "TXXX:MusicBrainz Release Group Id")
		mbArtistID = getRawTag(raw, "TXXX:MusicBrainz Artist Id")
		mbAlbumArtistID = getRawTag(raw, "TXXX:MusicBrainz Album Artist Id")

		// UFID frame is the canonical location for Recording ID in ID3v2
		if mbRecordingID == "" {
			mbRecordingID = getRawTag(raw, "UFID:http://musicbrainz.org")
		}
	}

	return MusicBrainzTags{
		MusicBrainzTrackID:        mbTrackID,
		MusicBrainzRecordingID:    mbRecordingID,
		MusicBrainzReleaseID:      mbReleaseID,
		MusicBrainzReleaseGroupID: mbReleaseGroupID,
		MusicBrainzArtistID:       mbArtistID,
		MusicBrainzAlbumArtistID:  mbAlbumArtistID,
	}
}
