package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var rootDir string

type FileEntry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime time.Time
	Ext     string
}

type TreeNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	Children []*TreeNode `json:"children,omitempty"`
}

type Breadcrumb struct {
	Name string
	Path string
}

type PageData struct {
	CurrentPath string
	Breadcrumbs []Breadcrumb
	Files       []FileEntry
	TreeJSON    template.JS
}

const tmpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{ .CurrentPath }}</title>
<style>
*,::after,::before{box-sizing:border-box;margin:0;padding:0}
a:hover{text-decoration:underline}
body,html{height:100%;overflow:hidden}
body{background:#1a1a1a;color:#d4d4d4;font-family:Consolas,Menlo,'DejaVu Sans Mono',monospace;font-size:13px;display:flex;flex-direction:column}
footer{padding:6px 16px;color:#777}
header{border-bottom:1px solid #333;padding:10px 16px;display:flex;align-items:center;gap:12px}
@media (max-width: 640px) { .sidebar { display: none; } .file-table td.date { display: none; } .file-table td.size { min-width: 50px; } .thumb-grid { grid-template-columns: repeat(auto-fill, minmax(120px, 1fr)); gap: 8px; padding: 8px; } }
.breadcrumb .current{color:#777}
.breadcrumb .sep{color:#777;margin:0 2px}
.breadcrumb a,.file-table thead th:hover,.thumb-label a{color:#d4d4d4}
.breadcrumb a:hover,.thumb-label a:hover,a{color:#5b9bd5;text-decoration:none}
.breadcrumb,.tree-item,.view-toggle{display:flex;gap:4px}
.breadcrumb{align-items:center;flex-wrap:wrap;flex:1}
.empty{padding:48px;text-align:center;color:#777}
.file-icon.dir,.thumb-preview .dir-icon{color:#c8a94e}
.file-icon.img{color:#7cbf7c}
.file-icon{margin-right:6px;color:#777}
.file-table tbody tr,.file-table thead th{border-bottom:1px solid #333}
.file-table tbody tr:hover,.tree-item.active,.tree-item:hover{background:#2e2e2e}
.file-table td.date{color:#777;min-width:130px}
.file-table td.name{width:100%;white-space:normal;word-break:break-all}
.file-table td.size{color:#777;text-align:right;min-width:70px}
.file-table td{padding:7px 12px;white-space:nowrap}
.file-table thead th,.tree-item{cursor:pointer;user-select:none;white-space:nowrap}
.file-table thead th.sort-asc::after{content:' ↑';color:#5b9bd5}
.file-table thead th.sort-desc::after{content:' ↓';color:#5b9bd5}
.file-table thead th{background:#222;padding:8px 12px;text-align:left;font-weight:400;color:#777;font-size:11px;letter-spacing:.08em;text-transform:uppercase}
.file-table thead{position:sticky;top:0}
.file-table{width:100%;border-collapse:collapse}
.layout{display:flex;flex:1;overflow:hidden}
.main{flex:1;overflow-y:auto}
.sidebar,.thumb-item,footer,header{background:#222}
.sidebar{width:220px;min-width:220px;border-right:1px solid #333;overflow-y:auto;overflow-x:hidden;padding:8px 0;flex-shrink:0}
.sidebar-title{font-size:10px;letter-spacing:.1em;color:#777;padding:4px 12px 8px;text-transform:uppercase}
.thumb-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(160px,1fr));gap:12px;padding:16px}
.thumb-item:hover{border-color:#5b9bd5}
.thumb-item{border:1px solid #333;overflow:hidden}
.thumb-label,footer{border-top:1px solid #333;font-size:11px}
.thumb-label{padding:6px 8px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.thumb-meta{font-size:10px;color:#777;padding:0 8px 6px}
.thumb-preview .big-icon{font-size:36px;color:#777}
.thumb-preview img{width:100%;height:100%;object-fit:cover}
.thumb-preview{width:100%;height:110px;display:flex;align-items:center;justify-content:center;background:#2a2a2a;overflow:hidden}
.tree-children.open{display:block}
.tree-children{display:none;padding-left:12px}
.tree-icon{color:#777;flex-shrink:0}
.tree-item.active{color:#5b9bd5}
.tree-item{align-items:center;padding:3px 8px;overflow:hidden;color:#d4d4d4;font-size:12px}
.tree-label{overflow:hidden;text-overflow:ellipsis}
.tree-root{list-style:none}
.tree-toggle{width:12px;text-align:center;color:#777;flex-shrink:0;font-size:10px}
.view-toggle button.active{background:#1a1a1a;color:#5b9bd5;border-color:#5b9bd5}
.view-toggle button:hover{background:#2e2e2e;color:#d4d4d4}
.view-toggle button{background:#2a2a2a;border:1px solid #333;color:#777;padding:4px 10px;cursor:pointer;font-size:12px;font-family:inherit}
.view-toggle{margin-left:auto}
</style>
</head>
<body>

<header>
  <span>&#128193;</span>
  <div class="breadcrumb">
    {{ range $i, $crumb := .Breadcrumbs }}
      {{ if gt $i 0 }}<span class="sep">/</span>{{ end }}
      {{ if eq $i (sub1 (len $.Breadcrumbs)) }}
        <span class="current">{{ $crumb.Name }}</span>
      {{ else }}
        <a href="{{ $crumb.Path }}">{{ $crumb.Name }}</a>
      {{ end }}
    {{ end }}
  </div>
  <div class="view-toggle">
    <button id="btn-list" class="active" onclick="setView('list')">&#9776; List</button>
    <button id="btn-thumb" onclick="setView('thumb')">&#9638; Thumbs</button>
  </div>
</header>

<div class="layout">
  <nav class="sidebar">
    <div class="sidebar-title">Folders</div>
    <ul class="tree-root" id="tree-root"></ul>
  </nav>

  <main class="main">
    <div id="view-list">
      {{ if .Files }}
      <table class="file-table" id="file-table">
        <thead>
          <tr>
            <th class="name" onclick="sortTable(0)">Name</th>
            <th class="size" onclick="sortTable(1)">Size</th>
            <th class="date" onclick="sortTable(2)">Modified</th>
          </tr>
        </thead>
        <tbody>
          {{ if ne .CurrentPath "/" }}
          <tr>
            <td class="name"><span class="file-icon dir">&#128193;</span><a href="../">../</a></td>
            <td class="size">—</td>
            <td class="date">—</td>
          </tr>
          {{ end }}
          {{ range .Files }}
          <tr data-name="{{ .Name }}" data-size="{{ .Size }}" data-date="{{ .ModTime.Unix }}">
            <td class="name">
              {{ if .IsDir }}
                <span class="file-icon dir">&#128193;</span><a href="{{ .Path }}">{{ .Name }}/</a>
              {{ else if isImage .Ext }}
                <span class="file-icon img">&#128247;</span><a href="{{ .Path }}">{{ .Name }}</a>
              {{ else }}
                <span class="file-icon">&#128196;</span><a href="{{ .Path }}">{{ .Name }}</a>
              {{ end }}
            </td>
            <td class="size">{{ if .IsDir }}—{{ else }}{{ formatSize .Size }}{{ end }}</td>
            <td class="date">{{ formatTime .ModTime }}</td>
          </tr>
          {{ end }}
        </tbody>
      </table>
      {{ else }}
      <div class="empty">Empty directory</div>
      {{ end }}
    </div>

    <div id="view-thumb" style="display:none">
      {{ if .Files }}
      <div class="thumb-grid">
        {{ if ne .CurrentPath "/" }}
        <div class="thumb-item">
          <a href="../" style="display:block">
            <div class="thumb-preview"><span class="big-icon dir-icon">&#8593;</span></div>
            <div class="thumb-label">../</div>
          </a>
        </div>
        {{ end }}
        {{ range .Files }}
        <div class="thumb-item">
          <a href="{{ .Path }}" style="display:block">
            <div class="thumb-preview">
              {{ if .IsDir }}
                <span class="big-icon dir-icon">&#128193;</span>
              {{ else if isImage .Ext }}
                <img src="/__thumb{{ .Path }}" alt="{{ .Name }}" loading="lazy">
              {{ else }}
                <span class="big-icon">&#128196;</span>
              {{ end }}
            </div>
            <div class="thumb-label" title="{{ .Name }}">{{ .Name }}{{ if .IsDir }}/{{ end }}</div>
            <div class="thumb-meta">{{ if .IsDir }}folder{{ else }}{{ formatSize .Size }}{{ end }}</div>
          </a>
        </div>
        {{ end }}
      </div>
      {{ else }}
      <div class="empty">Empty directory</div>
      {{ end }}
    </div>
  </main>
</div>

<footer>{{ len .Files }} item(s) &nbsp;·&nbsp; {{ .CurrentPath }}</footer>

<script>
const TREE_DATA = {{ .TreeJSON }};
const CURRENT_PATH = {{ printf "%q" .CurrentPath }};

function setView(v) {
  const isList = v === 'list';
  document.getElementById('view-list').style.display = isList ? '' : 'none';
  document.getElementById('view-thumb').style.display = isList ? 'none' : '';
  document.getElementById('btn-list').className = isList ? 'active' : '';
  document.getElementById('btn-thumb').className = isList ? '' : 'active';
}

let sortCol = -1, sortDir = 1;
function sortTable(col) {
  const tbody = document.querySelector('#file-table tbody');
  const rows = Array.from(tbody.querySelectorAll('tr'));
  const parentRow = rows.find(r => r.querySelector('a[href="../"]'));
  const dataRows = rows.filter(r => r !== parentRow);

  if (sortCol === col) sortDir *= -1;
  else { sortCol = col; sortDir = 1; }

  dataRows.sort((a, b) => {
    let av, bv;
    if (col === 0) { av = (a.dataset.name||'').toLowerCase(); bv = (b.dataset.name||'').toLowerCase(); }
    else if (col === 1) { av = parseInt(a.dataset.size||0); bv = parseInt(b.dataset.size||0); }
    else { av = parseInt(a.dataset.date||0); bv = parseInt(b.dataset.date||0); }
    return av < bv ? -sortDir : av > bv ? sortDir : 0;
  });

  document.querySelectorAll('#file-table thead th').forEach((th, i) => {
    th.className = th.className.replace(/ ?sort-(asc|desc)/g, '');
    if (i === col) th.className += sortDir > 0 ? ' sort-asc' : ' sort-desc';
  });

  tbody.innerHTML = '';
  if (parentRow) tbody.appendChild(parentRow);
  dataRows.forEach(r => tbody.appendChild(r));
}

function buildTree(nodes, container, depth) {
  if (!nodes || !nodes.length) return;
  nodes.forEach(node => {
    const li = document.createElement('li');
    const item = document.createElement('div');
    item.className = 'tree-item' + (node.path === CURRENT_PATH ? ' active' : '');
    item.style.paddingLeft = (8 + depth * 12) + 'px';

    const toggle = document.createElement('span');
    toggle.className = 'tree-toggle';
    toggle.textContent = (node.children && node.children.length) ? '▶' : ' ';

    const icon = document.createElement('span');
    icon.className = 'tree-icon';
    icon.textContent = '📁';

    const label = document.createElement('span');
    label.className = 'tree-label';
    label.textContent = node.name;

    item.append(toggle, icon, label);
    li.appendChild(item);

    let childUl = null;
    if (node.children && node.children.length) {
      childUl = document.createElement('ul');
      childUl.className = 'tree-children';
      buildTree(node.children, childUl, depth + 1);
      li.appendChild(childUl);
      toggle.addEventListener('click', e => {
        e.stopPropagation();
        toggle.textContent = childUl.classList.toggle('open') ? '▼' : '▶';
      });
    }

    item.addEventListener('click', () => { window.location.href = node.path; });

    if (CURRENT_PATH.startsWith(node.path) && childUl) {
      childUl.classList.add('open');
      toggle.textContent = '▼';
    }

    container.appendChild(li);
  });
}

buildTree(TREE_DATA, document.getElementById('tree-root'), 0);
</script>
</body>
</html>`

func main() {
	var dir string
	var port int
	flag.StringVar(&dir, "d", ".", "Directory to serve")
	flag.IntVar(&port, "p", 8080, "Port to listen on")
	flag.Parse()

	absDir, err := filepath.Abs(dir)
	if err != nil {
		log.Fatalf("Invalid directory: %v", err)
	}
	if _, err := os.Stat(absDir); os.IsNotExist(err) {
		log.Fatalf("Directory does not exist: %s", absDir)
	}

	rootDir = absDir
	log.Printf("Serving %s on :%d", rootDir, port)

	http.HandleFunc("/__thumb/", thumbHandler)
	http.HandleFunc("/", listingHandler)

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func safeFS(urlPath string) (string, bool) {
	fsPath := filepath.Join(rootDir, filepath.FromSlash(urlPath))
	clean := filepath.Clean(fsPath)
	rootClean := filepath.Clean(rootDir)
	return clean, strings.HasPrefix(clean, rootClean+string(os.PathSeparator)) || clean == rootClean
}

func listingHandler(w http.ResponseWriter, r *http.Request) {
	urlPath := r.URL.Path
	if urlPath == "" {
		urlPath = "/"
	}

	fsPath, ok := safeFS(urlPath)
	if !ok {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	info, err := os.Stat(fsPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if !info.IsDir() {
		http.ServeFile(w, r, fsPath)
		return
	}

	if !strings.HasSuffix(r.URL.Path, "/") {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
		return
	}

	entries, err := os.ReadDir(fsPath)
	if err != nil {
		http.Error(w, "Cannot read directory", http.StatusInternalServerError)
		return
	}

	var files []FileEntry
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		entryPath := urlPath + name
		if e.IsDir() {
			entryPath += "/"
		}
		ext := ""
		if !e.IsDir() {
			ext = strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
		}
		files = append(files, FileEntry{
			Name:    name,
			Path:    entryPath,
			IsDir:   e.IsDir(),
			Size:    fi.Size(),
			ModTime: fi.ModTime(),
			Ext:     ext,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	tree := buildTree(rootDir, "/")
	treeBytes, _ := json.Marshal(tree)

	data := PageData{
		CurrentPath: urlPath,
		Breadcrumbs: buildBreadcrumbs(urlPath),
		Files:       files,
		TreeJSON:    template.JS(treeBytes),
	}

	funcMap := template.FuncMap{
		"formatSize": formatSize,
		"formatTime": func(t time.Time) string { return t.Format("2006-01-02 15:04") },
		"isImage":    isImageExt,
		"sub1":       func(n int) int { return n - 1 },
	}

	t, err := template.New("page").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil {
		log.Printf("Template error: %v", err)
	}
}

func thumbHandler(w http.ResponseWriter, r *http.Request) {
	urlPath := strings.TrimPrefix(r.URL.Path, "/__thumb")
	fsPath, ok := safeFS(urlPath)
	if !ok {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	f, err := os.Open(fsPath)
	if err != nil {
		servePlaceholder(w)
		return
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		servePlaceholder(w)
		return
	}

	thumb := resizeNN(img, 200, 150)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "max-age=3600")
	jpeg.Encode(w, thumb, &jpeg.Options{Quality: 75})
}

func resizeNN(src image.Image, maxW, maxH int) image.Image {
	b := src.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	scale := float64(maxW) / float64(srcW)
	if s := float64(maxH) / float64(srcH); s < scale {
		scale = s
	}
	newW := int(float64(srcW)*scale + 0.5)
	newH := int(float64(srcH)*scale + 0.5)
	if newW < 1 { newW = 1 }
	if newH < 1 { newH = 1 }
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	for y := 0; y < newH; y++ {
		for x := 0; x < newW; x++ {
			dst.Set(x, y, src.At(b.Min.X+int(float64(x)/scale), b.Min.Y+int(float64(y)/scale)))
		}
	}
	return dst
}

func servePlaceholder(w http.ResponseWriter) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 150))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{40, 40, 40, 255}}, image.Point{}, draw.Src)
	w.Header().Set("Content-Type", "image/jpeg")
	jpeg.Encode(w, img, &jpeg.Options{Quality: 60})
}

func buildBreadcrumbs(urlPath string) []Breadcrumb {
	crumbs := []Breadcrumb{{Name: "root", Path: "/"}}
	cur := "/"
	for _, p := range strings.Split(strings.Trim(urlPath, "/"), "/") {
		if p == "" { continue }
		cur = cur + p + "/"
		crumbs = append(crumbs, Breadcrumb{Name: p, Path: cur})
	}
	return crumbs
}

func buildTree(basePath, urlBase string) []*TreeNode {
	entries, err := os.ReadDir(basePath)
	if err != nil { return nil }
	var nodes []*TreeNode
	for _, e := range entries {
		if !e.IsDir() { continue }
		name := e.Name()
		if strings.HasPrefix(name, ".") { continue }
		nodePath := urlBase + name + "/"
		node := &TreeNode{Name: name, Path: nodePath}
		node.Children = buildTree(filepath.Join(basePath, name), nodePath)
		nodes = append(nodes, node)
	}
	return nodes
}

func isImageExt(ext string) bool {
	switch ext {
	case "jpg", "jpeg", "png", "gif", "webp", "bmp", "avif":
		return true
	}
	return false
}

func formatSize(n int64) string {
	if n < 1024 { return fmt.Sprintf("%d B", n) }
	div, exp := int64(1024), 0
	for v := n / 1024; v >= 1024; v /= 1024 { div *= 1024; exp++ }
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
