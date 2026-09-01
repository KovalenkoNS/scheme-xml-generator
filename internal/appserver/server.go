package appserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"scheme-xml-generator/internal/generator"
	"scheme-xml-generator/internal/library"
)

type Server struct {
	repository *library.Repository
	generator  generator.Generator
	allocator  *generator.Allocator
	outputDir  string
	staticFS   fs.FS
	logger     *log.Logger
	mux        *http.ServeMux
	outputMu   sync.Mutex
}

type generateResponse struct {
	FileName string            `json:"fileName"`
	URL      string            `json:"url"`
	BaseName string            `json:"baseName"`
	Summary  generator.Summary `json:"summary"`
	Warnings []string          `json:"warnings"`
}

type outputFile struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"createdAt"`
	URL       string `json:"url"`
}

func New(repository *library.Repository, xmlGenerator generator.Generator, allocator *generator.Allocator, outputDir string, staticFS fs.FS, logger *log.Logger) *Server {
	server := &Server{repository: repository, generator: xmlGenerator, allocator: allocator, outputDir: outputDir, staticFS: staticFS, logger: logger, mux: http.NewServeMux()}
	server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return securityHeaders(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	s.mux.HandleFunc("GET /api/templates", s.handleTemplates)
	s.mux.HandleFunc("POST /api/refresh", s.localPOST(s.handleRefresh))
	s.mux.HandleFunc("POST /api/preview-name", s.localPOST(s.handlePreviewName))
	s.mux.HandleFunc("POST /api/generate", s.localPOST(s.handleGenerate))
	s.mux.HandleFunc("GET /api/outputs", s.handleOutputs)
	s.mux.HandleFunc("GET /api/output/{name}", s.handleDownload)
	s.mux.Handle("GET /", http.FileServer(http.FS(s.staticFS)))
}

func (s *Server) handleTemplates(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.repository.Catalog())
}

func (s *Server) handleRefresh(w http.ResponseWriter, _ *http.Request) {
	catalog, err := s.repository.Refresh()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, catalog)
}

func (s *Server) handlePreviewName(w http.ResponseWriter, r *http.Request) {
	var request struct {
		TemplateKey string `json:"templateKey"`
		ObjectName  string `json:"objectName"`
		NameMode    string `json:"nameMode"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ref, ok := s.repository.Resolve(request.TemplateKey)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("шаблон не найден; обновите список библиотек"))
		return
	}
	preview, err := generator.PreviewName(ref, request.ObjectName, request.NameMode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	var request generator.Request
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ref, ok := s.repository.Resolve(request.TemplateKey)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("шаблон не найден; обновите список библиотек"))
		return
	}
	t11Count, cardCount := generator.Requirements(ref)
	ids, err := s.allocator.Reserve(t11Count, cardCount)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("выделить диапазон ID: %w", err))
		return
	}
	result, err := s.generator.Generate(ref, request, ids)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	fileName := safeOutputName(request.FileName, result.BaseName, ref.Template.Name)
	s.outputMu.Lock()
	defer s.outputMu.Unlock()
	fileName, err = uniqueOutputName(s.outputDir, fileName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := writeAtomic(filepath.Join(s.outputDir, fileName), result.XML); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("записать результат: %w", err))
		return
	}
	s.logger.Printf("generated %s from %s/%s for %s", fileName, ref.Library.FileName, ref.Template.Name, result.BaseName)
	writeJSON(w, http.StatusCreated, generateResponse{FileName: fileName, URL: "/api/output/" + url.PathEscape(fileName), BaseName: result.BaseName, Summary: result.Summary, Warnings: result.Warnings})
}

func (s *Server) handleOutputs(w http.ResponseWriter, _ *http.Request) {
	entries, err := os.ReadDir(s.outputDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	files := make([]outputFile, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".xml") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, outputFile{Name: entry.Name(), Size: info.Size(), CreatedAt: info.ModTime().Format(time.RFC3339), URL: "/api/output/" + url.PathEscape(entry.Name())})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].CreatedAt > files[j].CreatedAt })
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	name, err := url.PathUnescape(r.PathValue("name"))
	if err != nil || filepath.Base(name) != name || !strings.EqualFold(filepath.Ext(name), ".xml") {
		writeError(w, http.StatusBadRequest, fmt.Errorf("недопустимое имя файла"))
		return
	}
	path := filepath.Join(s.outputDir, name)
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, fmt.Errorf("файл не найден"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	_, _ = io.Copy(w, file)
}

func (s *Server) localPOST(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			parsed, err := url.Parse(origin)
			host := strings.Split(parsed.Host, ":")[0]
			if err != nil || (host != "127.0.0.1" && host != "localhost") {
				writeError(w, http.StatusForbidden, fmt.Errorf("запрос отклонён проверкой Origin"))
				return
			}
		}
		next(w, r)
	}
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("неверный JSON: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("после JSON обнаружены лишние данные")
	}
	return nil
}

func safeOutputName(requested, baseName, templateName string) string {
	name := strings.TrimSpace(requested)
	if name == "" {
		name = baseName + "_" + templateName
	}
	name = strings.TrimSuffix(name, filepath.Ext(name))
	var builder strings.Builder
	lastSeparator := false
	for _, r := range name {
		valid := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'
		if !valid {
			r = '_'
		}
		if r == '_' && lastSeparator {
			continue
		}
		builder.WriteRune(r)
		lastSeparator = r == '_'
		if builder.Len() >= 120 {
			break
		}
	}
	name = strings.Trim(builder.String(), "_.-")
	if name == "" {
		name = "generated_scheme"
	}
	return name + ".xml"
}

func uniqueOutputName(directory, name string) (string, error) {
	extension := filepath.Ext(name)
	base := strings.TrimSuffix(name, extension)
	for index := 1; index < 10000; index++ {
		candidate := name
		if index > 1 {
			candidate = fmt.Sprintf("%s-%d%s", base, index, extension)
		}
		_, err := os.Stat(filepath.Join(directory, candidate))
		if os.IsNotExist(err) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("не удалось подобрать свободное имя результата")
}

func writeAtomic(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(directory, ".generating-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func Shutdown(ctx context.Context, server *http.Server) error {
	return server.Shutdown(ctx)
}
