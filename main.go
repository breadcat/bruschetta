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

	tmpl, err := template.New("listing.html").Funcs(funcMap).ParseFiles("templates/listing.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
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
