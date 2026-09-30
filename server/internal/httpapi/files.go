package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
)

var AllowType = map[string]bool{
	"flac": true, "mp3": true, "ape": true, "wav": true, "aiff": true,
	"wv": true, "tta": true, "m4a": true, "ogg": true, "mpc": true,
	"opus": true, "wma": true, "dsf": true, "dff": true, "wmv": true,
}

// batchUpdateAllowType is the slightly shorter list used by batch_update_id3
// in the original views module.
var batchUpdateAllowType = map[string]bool{
	"flac": true, "mp3": true, "ape": true, "wav": true, "aiff": true,
	"wv": true, "tta": true, "m4a": true, "ogg": true, "mpc": true,
	"opus": true, "wma": true, "dsf": true, "dff": true,
}

type fileNode struct {
	ID         int         `json:"id"`
	Name       string      `json:"name"`
	Title      string      `json:"title"`
	Icon       string      `json:"icon,omitempty"`
	State      string      `json:"state,omitempty"`
	Children   interface{} `json:"children,omitempty"`
	Expanded   bool        `json:"expanded,omitempty"`
	Size       int64       `json:"size"`
	UpdateTime string      `json:"update_time"`
}

type fileEntry struct {
	Name       string
	Path       string
	IsDir      bool
	Size       int64
	UpdateTime string
}

// HandleFileList replicates POST /api/file_list/ exactly, including the GBK
// byte-order name sort the original applied to CJK filenames.
func (s *Server) HandleFileList(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FilePath     string   `json:"file_path"`
		SortedFields []string `json:"sorted_fields"`
	}
	if err := DecodeJSON(r, &body); err != nil || body.FilePath == "" {
		Failure(w, "参数错误")
		return
	}
	filePath := strings.TrimSuffix(body.FilePath, "/")
	dirEntries, err := os.ReadDir(filePath)
	if err != nil {
		Failure(w, "文件夹不存在")
		return
	}

	var fileData []fileEntry
	lrcMap := map[string]bool{}
	for _, entry := range dirEntries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		name := entry.Name()
		fileData = append(fileData, fileEntry{
			Name:       name,
			Path:       filePath + "/" + name,
			IsDir:      entry.IsDir(),
			Size:       info.Size(),
			UpdateTime: info.ModTime().Format("2006-01-02 15:04:05"),
		})
		ext := strings.ToLower(extOf(name))
		if ext == "lrc" || ext == "txt" {
			lrcMap[stemOf(name)] = true
		}
	}
	taskMap, _ := s.Store.TaskStateMap(filePath)

	var children []fileNode
	for _, entry := range fileData {
		ext := extOf(entry.Name)
		if entry.IsDir {
			children = append(children, fileNode{
				Name: entry.Name, Title: entry.Name,
				Icon: "icon-folder", State: "null", Children: []any{},
				Size: entry.Size, UpdateTime: entry.UpdateTime,
			})
			continue
		}
		if !AllowType[strings.ToLower(ext)] {
			continue
		}
		icon := "icon-script-file"
		if lrcMap[stemOf(entry.Name)] {
			icon = "icon-script-files"
		}
		state := "null"
		if v, ok := taskMap[entry.Name]; ok {
			state = v
		}
		children = append(children, fileNode{
			Name: entry.Name, Title: entry.Name,
			Icon: icon, State: state,
			Size: entry.Size, UpdateTime: entry.UpdateTime,
		})
	}
	// Python numbered ids with enumerate() over every entry, not just children.
	for i := range children {
		children[i].ID = i + 1
	}

	switch {
	case contains(body.SortedFields, "name"):
		sort.SliceStable(children, func(i, j int) bool {
			return gbkLess(children[i].Name, children[j].Name)
		})
	case contains(body.SortedFields, "update_time"):
		sort.SliceStable(children, func(i, j int) bool {
			return children[i].UpdateTime > children[j].UpdateTime
		})
	case contains(body.SortedFields, "size"):
		sort.SliceStable(children, func(i, j int) bool {
			return children[i].Size > children[j].Size
		})
	}

	rootName := filepath.Base(strings.TrimSuffix(body.FilePath, "/"))
	root := []fileNode{{
		ID: 0, Name: rootName, Title: rootName, Expanded: true,
		Children: children, Icon: "icon-folder",
	}}
	Success(w, root)
}

func extOf(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return ""
}

func stemOf(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[:i]
	}
	return name
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// gbkLess compares names by GBK bytes like the Python `name.encode('gbk', 'ignore')` sort.
func gbkLess(a, b string) bool {
	gbk := simplifiedchinese.GBK.NewEncoder()
	ab, err1 := gbk.Bytes([]byte(a))
	bb, err2 := gbk.Bytes([]byte(b))
	if err1 != nil {
		ab = []byte(a)
	}
	if err2 != nil {
		bb = []byte(b)
	}
	return string(ab) < string(bb)
}
