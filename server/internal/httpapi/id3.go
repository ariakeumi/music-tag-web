package httpapi

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// HandleMusicID3 replicates POST /api/music_id3/.
func (s *Server) HandleMusicID3(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FilePath string `json:"file_path"`
		FileName string `json:"file_name"`
	}
	if err := DecodeJSON(r, &body); err != nil {
		Failure(w, "参数错误")
		return
	}
	ext := strings.ToLower(extOf(body.FileName))
	if ext == "lrc" || ext == "txt" {
		Success(w, nil)
		return
	}
	filePath := strings.TrimSuffix(body.FilePath, "/")
	if filepath.Base(filePath) == body.FileName {
		Success(w, nil)
		return
	}
	resolved, err := resolveMusicFile(filePath, body.FileName)
	if err != nil {
		Failure(w, err.Error())
		return
	}
	data, err := s.Tags.Read(resolved)
	if err != nil {
		Failure(w, err.Error())
		return
	}
	Success(w, data)
}

// HandleUpdateID3 replicates POST /api/update_id3/.
func (s *Server) HandleUpdateID3(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MusicID3Info []map[string]any `json:"music_id3_info"`
	}
	if err := DecodeJSON(r, &body); err != nil {
		Failure(w, "参数错误")
		return
	}
	if err := s.writeID3(body.MusicID3Info); err != nil {
		Failure(w, err.Error())
		return
	}
	Success(w, nil)
}

// writeID3 delegates to the Python tag CLI and records a success Task row
// per updated file, mirroring update_music_info. Missing paths are resolved
// by unique basename under their directory (stale tree paths from the
// recursive file list never silently address the wrong file).
func (s *Server) writeID3(items []map[string]any) error {
	for _, item := range items {
		fullPath, _ := item["file_full_path"].(string)
		if fullPath == "" {
			continue
		}
		if _, err := os.Stat(fullPath); err != nil {
			resolved, err := resolveMusicFile(filepath.Dir(fullPath), filepath.Base(fullPath))
			if err != nil {
				continue
			}
			item["file_full_path"] = resolved
		}
	}
	results, err := s.Tags.Write(items)
	if err != nil {
		return err
	}
	var failures []string
	for _, res := range results {
		if !res.OK {
			failures = append(failures, res.Error)
			continue
		}
		_ = s.Store.UpsertTask(res.FullPath, "success",
			filepath.Dir(res.FullPath), filepath.Base(res.FullPath), "", "")
	}
	if len(failures) > 0 {
		return joinErrors(failures)
	}
	return nil
}

// HandleBatchUpdateID3 replicates POST /api/batch_update_id3/: expand
// selected folders into per-file payloads, then write them all.
func (s *Server) HandleBatchUpdateID3(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FullPath   string         `json:"file_full_path"`
		SelectData []selectItem   `json:"select_data"`
		MusicInfo  map[string]any `json:"music_info"`
	}
	if err := DecodeJSON(r, &body); err != nil {
		Failure(w, "参数错误")
		return
	}
	var items []map[string]any
	for _, sel := range body.SelectData {
		if sel.Icon == "icon-folder" {
			dir := body.FullPath + "/" + sel.Name
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if !batchUpdateAllowType[strings.ToLower(extOf(entry.Name()))] {
					continue
				}
				item := deepCopyMap(body.MusicInfo)
				item["file_full_path"] = dir + "/" + entry.Name()
				item["filename"] = entry.Name()
				items = append(items, item)
			}
		} else {
			item := deepCopyMap(body.MusicInfo)
			item["file_full_path"] = body.FullPath + "/" + sel.Name
			items = append(items, item)
		}
	}
	if err := s.writeID3(items); err != nil {
		Failure(w, err.Error())
		return
	}
	Success(w, nil)
}

// HandleUploadImage replicates POST /api/upload_image/: multipart file in,
// base64 string out.
func (s *Server) HandleUploadImage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		Failure(w, "参数错误")
		return
	}
	file, _, err := r.FormFile("upload_file")
	if err != nil {
		Failure(w, "未找到上传文件")
		return
	}
	defer file.Close()
	buf := make([]byte, 0, 1<<20)
	tmp := make([]byte, 32*1024)
	for {
		n, err := file.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	Success(w, base64.StdEncoding.EncodeToString(buf))
}

type selectItem struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
}

func deepCopyMap(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func joinErrors(errs []string) error {
	return &multiError{errs}
}

type multiError struct{ errs []string }

func (m *multiError) Error() string {
	return strings.Join(m.errs, "; ")
}
