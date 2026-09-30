package httpapi

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xhongc/music-tag-web/server/internal/store"
)

type selectItemQ = selectItem

// HandleBatchAutoUpdateID3 replicates POST /api/batch_auto_update_id3/:
// create TaskRecord rows for the selection, then start a background job.
func (s *Server) HandleBatchAutoUpdateID3(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FullPath   string         `json:"file_full_path"`
		SelectData []selectItemQ  `json:"select_data"`
		MusicInfo  map[string]any `json:"music_info"`
	}
	if err := DecodeJSON(r, &body); err != nil {
		Failure(w, "参数错误")
		return
	}
	selectMode, _ := body.MusicInfo["select_mode"].(string)
	sourceListRaw, _ := body.MusicInfo["source_list"].([]any)
	var sourceList []string
	for _, src := range sourceListRaw {
		if v, ok := src.(string); ok {
			sourceList = append(sourceList, v)
		}
	}

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	var records []store.TaskRecord
	for _, sel := range body.SelectData {
		name := sel.Name
		records = append(records, store.TaskRecord{
			SongName: stemOf(name),
			FullPath: body.FullPath + "/" + name,
			Icon:     sel.Icon,
			Batch:    timestamp,
		})
	}
	if err := s.Store.CreateTaskRecords(records); err != nil {
		Failure(w, err.Error())
		return
	}
	svc := s.TagTask
	s.Queue.Submit("batch_auto_tag", func(ctx context.Context) {
		svc.BatchAutoTag(ctx, timestamp, sourceList, selectMode)
	})
	Success(w, nil)
}

// HandleTidyFolder replicates POST /api/tidy_folder/.
func (s *Server) HandleTidyFolder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RootPath   string        `json:"root_path"`
		FirstDir   string        `json:"first_dir"`
		FullPath   string        `json:"file_full_path"`
		SelectData []selectItemQ `json:"select_data"`
		SecondDir  string        `json:"second_dir"`
	}
	if err := DecodeJSON(r, &body); err != nil {
		Failure(w, "参数错误")
		return
	}
	var paths []string
	for _, sel := range body.SelectData {
		if sel.Icon == "icon-folder" {
			dir := body.FullPath + "/" + sel.Name
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if !AllowType[lower(extensionOf(entry.Name()))] {
					continue
				}
				paths = append(paths, dir+"/"+entry.Name())
			}
		} else {
			paths = append(paths, body.FullPath+"/"+sel.Name)
		}
	}
	svc := s.TagTask
	rootPath, firstDir, secondDir := body.RootPath, body.FirstDir, body.SecondDir
	s.Queue.Submit("tidy_folder", func(ctx context.Context) {
		_ = svc.TidyFolder(ctx, paths, rootPath, firstDir, secondDir)
	})
	Success(w, nil)
}

// HandleRecord replicates GET /api/record/ with the DRF pagination envelope.
func (s *Server) HandleRecord(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize <= 0 {
		pageSize = 10
	}
	state := q.Get("state")

	tasks, total, err := s.Store.ListTasks(state, page, pageSize)
	if err != nil {
		Failure(w, err.Error())
		return
	}
	items := []any{}
	for _, t := range tasks {
		isExists := true
		if _, err := os.Stat(t.FullPath); os.IsNotExist(err) {
			isExists = false
			_ = s.Store.DeleteTask(t.ID)
		}
		items = append(items, map[string]any{
			"id":          t.ID,
			"song_name":   t.SongName,
			"artist_name": t.ArtistName,
			"full_path":   t.FullPath,
			"state":       t.State,
			"parent_path": t.ParentPath,
			"filename":    t.Filename,
			"created_at":  t.CreatedAt,
			"message":     fmt.Sprintf("【%s】 未找到标签或修改失败！", t.SongName),
			"is_exists":   isExists,
		})
	}
	totalPage := int(math.Ceil(float64(total) / float64(pageSize)))
	writeJSON(w, http.StatusOK, PageData{
		Page:      page,
		TotalPage: totalPage,
		Count:     total,
		Items:     items,
	})
}

// HandleActiveQueue replicates GET /api/active_queue/.
func (s *Server) HandleActiveQueue(w http.ResponseWriter, r *http.Request) {
	Success(w, s.Queue.Active())
}

// HandleClearQueue replicates GET /api/clear_celery/.
func (s *Server) HandleClearQueue(w http.ResponseWriter, r *http.Request) {
	s.Queue.CancelAll()
	Success(w, nil)
}

func extensionOf(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return name[i+1:]
		}
	}
	return ""
}

func lower(s string) string {
	return strings.ToLower(s)
}
