package httpapi

import (
	"io/fs"
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

const (
	maxTreeDepth = 32
	maxTreeNodes = 100000
)

// junkDirs are vendor metadata directories that never belong in a tag editor
// tree (Synology thumbnail/recycle metadata follows files into every folder).
var junkDirs = map[string]bool{
	"@eaDir": true,
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

// HandleFileList replicates POST /api/file_list/, including the GBK
// byte-order name sort the original applied to CJK filenames. When
// RecursiveFileList is on (the default) the response carries the whole
// subtree, so the UI search box matches files in any subdirectory.
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
	if _, err := os.Stat(filePath); err != nil {
		Failure(w, "文件夹不存在")
		return
	}

	sortKey := ""
	switch {
	case contains(body.SortedFields, "name"):
		sortKey = "name"
	case contains(body.SortedFields, "update_time"):
		sortKey = "update_time"
	case contains(body.SortedFields, "size"):
		sortKey = "size"
	}

	states := map[string]map[string]string{}
	if s.Config.RecursiveFileList {
		states, _ = s.Store.TaskStatesUnderPrefix(filePath)
	} else {
		states[filePath], _ = s.Store.TaskStateMap(filePath)
	}

	b := &treeBuilder{
		sortKey:   sortKey,
		states:    states,
		recursive: s.Config.RecursiveFileList,
		nextID:    1,
	}
	children := b.build(filePath, 0)
	if b.truncated {
		// surface truncation through the message while keeping the tree usable
		SuccessMsg(w, "文件过多，结果已截断", []fileNode{{
			ID: 0, Name: filepath.Base(filePath), Title: filepath.Base(filePath),
			Expanded: true, Children: children, Icon: "icon-folder",
		}})
		return
	}
	rootName := filepath.Base(filePath)
	root := []fileNode{{
		ID: 0, Name: rootName, Title: rootName, Expanded: true,
		Children: children, Icon: "icon-folder",
	}}
	Success(w, root)
}

type treeBuilder struct {
	sortKey   string
	states    map[string]map[string]string
	recursive bool
	nextID    int
	nodes     int
	truncated bool
}

// build lists one directory; in recursive mode it descends into
// subdirectories (symlinks are not followed, so cycles cannot occur).
func (b *treeBuilder) build(dirPath string, depth int) []fileNode {
	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		return []fileNode{}
	}

	var fileData []fileEntry
	lrcMap := map[string]bool{}
	for _, entry := range dirEntries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		name := entry.Name()
		if entry.IsDir() && junkDirs[name] {
			continue
		}
		fileData = append(fileData, fileEntry{
			Name:       name,
			Path:       dirPath + "/" + name,
			IsDir:      entry.IsDir(),
			Size:       info.Size(),
			UpdateTime: info.ModTime().Format("2006-01-02 15:04:05"),
		})
		ext := strings.ToLower(extOf(name))
		if ext == "lrc" || ext == "txt" {
			lrcMap[stemOf(name)] = true
		}
	}

	var children []fileNode
	taskStates := b.states[dirPath]
	for _, entry := range fileData {
		ext := extOf(entry.Name)
		if entry.IsDir {
			node := fileNode{
				Name: entry.Name, Title: entry.Name,
				Icon: "icon-folder", State: "null",
				Size: entry.Size, UpdateTime: entry.UpdateTime,
			}
			if b.recursive && depth < maxTreeDepth && b.nodes < maxTreeNodes {
				// expanded: bk-tree 只自动展开命中的节点本身；预先展开所有
				// 目录，搜索命中嵌套文件时才能立刻显示出来
				node.Expanded = true
				node.Children = b.build(entry.Path, depth+1)
			} else {
				node.Children = []any{}
			}
			b.assignID(&node)
			children = append(children, node)
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
		if v, ok := taskStates[entry.Name]; ok {
			state = v
		}
		node := fileNode{
			Name: entry.Name, Title: entry.Name,
			Icon: icon, State: state,
			Size: entry.Size, UpdateTime: entry.UpdateTime,
		}
		b.assignID(&node)
		children = append(children, node)
	}

	b.sortChildren(children)
	return children
}

func (b *treeBuilder) assignID(node *fileNode) {
	node.ID = b.nextID
	b.nextID++
	b.nodes++
	if b.nodes >= maxTreeNodes {
		b.truncated = true
	}
}

func (b *treeBuilder) sortChildren(children []fileNode) {
	switch b.sortKey {
	case "name":
		sort.SliceStable(children, func(i, j int) bool {
			return gbkLess(children[i].Name, children[j].Name)
		})
	case "update_time":
		sort.SliceStable(children, func(i, j int) bool {
			return children[i].UpdateTime > children[j].UpdateTime
		})
	case "size":
		sort.SliceStable(children, func(i, j int) bool {
			return children[i].Size > children[j].Size
		})
	}
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

// resolveMusicFile maps a (directory, filename) request to an actual file.
// The exact path wins; otherwise the filename is searched recursively under
// the directory and only an unambiguous single hit is accepted, so a stale
// tree path can never silently address the wrong file.
func resolveMusicFile(dir, name string) (string, error) {
	exact := dir + "/" + name
	if _, err := os.Stat(exact); err == nil {
		return exact, nil
	}
	var match string
	count := 0
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() == name {
			count++
			if count == 1 {
				match = p
			}
			if count > 1 {
				return fs.SkipAll
			}
		}
		return nil
	})
	switch count {
	case 0:
		return "", errNotFound
	case 1:
		return match, nil
	default:
		return "", errAmbiguous
	}
}

var (
	errNotFound  = &staticError{"文件不存在"}
	errAmbiguous = &staticError{"文件路径不唯一，请先点击进入所在目录后再操作"}
)

type staticError struct{ msg string }

func (e *staticError) Error() string { return e.msg }
