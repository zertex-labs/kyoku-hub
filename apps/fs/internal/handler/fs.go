package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
	"github.com/zertex-labs/kyoku-hub/apps/fs/internal/config"
	"go.uber.org/zap"
)

type FileData struct {
	RelativePath string    `json:"rpath"`
	ModTime      time.Time `json:"mt"`
}

type FileDataMap = map[string][]FileData

type FsHandler struct {
	logger *zap.Logger
	cfg    *config.Config
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

			if !info.IsDir() {
				if record[dir] == nil {
					record[dir] = make([]FileData, 0)
				}
				rdata := record[dir]
				relativePath, err := filepath.Rel(dir, path)
				if err != nil {
					log.Error("failed to get relative path for " + path)
				}
				record[dir] = append(rdata, FileData{
					ModTime:      info.ModTime(),
					RelativePath: relativePath,
				})
			}

			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	return record, nil
}

func (h *FsHandler) ProcessFile(filepath string) {

}
