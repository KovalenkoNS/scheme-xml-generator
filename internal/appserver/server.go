package appserver

import (
	"context"
	"encoding/json"
	"errors"
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
	s.mux.HandleFunc("GET /api/temporary/ao/profile", s.handleAOProfile)
	s.mux.HandleFunc("POST /api/temporary/ao/preview", s.localPOST(s.handleAOPreview))
	s.mux.HandleFunc("POST /api/temporary/ao/generate", s.localPOST(s.handleAOGenerate))
	s.mux.HandleFunc("POST /api/temporary/ao/generate-st", s.localPOST(s.handleAOSTGenerate))
	s.mux.HandleFunc("POST /api/temporary/ao/generate-diagnostic", s.localPOST(s.handleAODiagnosticGenerate))
	s.mux.HandleFunc("POST /api/temporary/diagnostic/preview", s.localPOST(s.handlePLCDiagnosticPreview))
	s.mux.HandleFunc("POST /api/temporary/diagnostic/generate", s.localPOST(s.handlePLCDiagnosticGenerate))
	s.mux.HandleFunc("POST /api/techobjects/preview", s.localPOST(s.handleTechObjectsPreview))
	s.mux.HandleFunc("POST /api/techobjects/generate", s.localPOST(s.handleTechObjectsGenerate))
	s.mux.HandleFunc("GET /api/skz/profile", s.handleSKZProfile)
	s.mux.HandleFunc("POST /api/skz/preview", s.localPOST(s.handleSKZPreview))
	s.mux.HandleFunc("POST /api/skz/generate", s.localPOST(s.handleSKZGenerate))
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
	if err := decodeJSON(w, r, &request); err != nil {
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
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if request.POUs != nil {
		s.handleDocumentGenerate(w, request)
		return
	}
	ref, ok := s.repository.Resolve(request.TemplateKey)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("шаблон не найден; обновите список библиотек"))
		return
	}
	if supported, warnings := library.TemplateCompatibility(ref); !supported {
		writeError(w, http.StatusBadRequest, fmt.Errorf("шаблон %q несовместим с генератором: %s", ref.Template.Name, strings.Join(warnings, "; ")))
		return
	}
	t11Count, cardCount := generator.Requirements(ref)
	if t11Count > maxDocumentObjects || cardCount > maxDocumentCards {
		writeError(w, http.StatusBadRequest, fmt.Errorf("шаблон слишком велик: не более %d графических объектов и %d карточек", maxDocumentObjects, maxDocumentCards))
		return
	}
	var result generator.Result
	_, err := s.allocator.WithReservation(t11Count, cardCount, 1, generator.ReservationOptions{
		T11Start: request.T11Start, CardStart: request.CardStart, POUIDs: []*int64{request.POUID},
	}, func(ids generator.IDRange) error {
		var generateErr error
		result, generateErr = s.generator.Generate(ref, request, ids)
		return generateErr
	})
	if err != nil {
		var persistenceErr *generator.AllocatorPersistenceError
		if errors.As(err, &persistenceErr) {
			writeError(w, http.StatusInternalServerError, err)
		} else {
			writeError(w, http.StatusBadRequest, err)
		}
		return
	}
	s.writeGeneratedResult(w, request.FileName, ref.Template.Name, result)
}

const (
	maxDocumentPOUs    = 128
	maxDocumentSignals = 4096
	maxDocumentModules = 4096
	maxDocumentObjects = 200000
	maxDocumentCards   = 100000
)

func (s *Server) handleDocumentGenerate(w http.ResponseWriter, request generator.Request) {
	normalized, err := generator.NormalizePOURequests(request.POUs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request.POUs = normalized
	if hasLegacyGenerateFields(request) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("при использовании поля pous настройки POU и сигналов должны находиться внутри соответствующей POU"))
		return
	}
	if len(request.POUs) > maxDocumentPOUs {
		writeError(w, http.StatusBadRequest, fmt.Errorf("один файл может содержать не более %d POU", maxDocumentPOUs))
		return
	}

	keys := make([]string, 0)
	signalCount := 0
	moduleCount := 0
	for pouIndex, pou := range request.POUs {
		if pou.IO != nil {
			moduleCount += len(pou.IO.Modules)
			if moduleCount > maxDocumentModules {
				writeError(w, http.StatusBadRequest, fmt.Errorf("один файл может содержать не более %d физических модулей", maxDocumentModules))
				return
			}
		}
		signals := generator.POUSignals(pou)
		if len(signals) == 0 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("POU %d не содержит сигналов", pouIndex+1))
			return
		}
		signalCount += len(signals)
		if signalCount > maxDocumentSignals {
			writeError(w, http.StatusBadRequest, fmt.Errorf("один файл может содержать не более %d сигналов", maxDocumentSignals))
			return
		}
		for signalIndex, signal := range signals {
			key := strings.TrimSpace(signal.TemplateKey)
			if key == "" {
				key = strings.TrimSpace(pou.DefaultTemplateKey)
			}
			if key == "" {
				writeError(w, http.StatusBadRequest, fmt.Errorf("POU %d, сигнал %d: шаблон не выбран", pouIndex+1, signalIndex+1))
				return
			}
			keys = append(keys, key)
		}
	}

	references, missing := s.repository.ResolveMany(keys)
	if len(missing) > 0 {
		writeError(w, http.StatusNotFound, fmt.Errorf("шаблоны не найдены; обновите список библиотек: %s", strings.Join(missing, ", ")))
		return
	}
	checked := make(map[string]struct{}, len(references))
	resolved := make([]generator.ResolvedPOU, 0, len(request.POUs))
	for pouIndex, pou := range request.POUs {
		signals := generator.POUSignals(pou)
		resolvedPOU := generator.ResolvedPOU{Request: pou, Signals: make([]generator.ResolvedSignal, 0, len(signals))}
		for signalIndex, signal := range signals {
			key := strings.TrimSpace(signal.TemplateKey)
			if key == "" {
				key = strings.TrimSpace(pou.DefaultTemplateKey)
			}
			ref := references[key]
			if _, exists := checked[key]; !exists {
				supported, warnings := library.TemplateCompatibility(ref)
				if !supported {
					writeError(w, http.StatusBadRequest, fmt.Errorf("POU %d, сигнал %d: шаблон %q несовместим с генератором: %s", pouIndex+1, signalIndex+1, ref.Template.Name, strings.Join(warnings, "; ")))
					return
				}
				checked[key] = struct{}{}
			}
			effectiveSignal := signal
			effectiveSignal.TemplateKey = key
			resolvedPOU.Signals = append(resolvedPOU.Signals, generator.ResolvedSignal{Request: effectiveSignal, Ref: ref})
		}
		resolved = append(resolved, resolvedPOU)
	}

	requirements, err := generator.RequirementsForDocument(resolved)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if requirements.T11Count > maxDocumentObjects || requirements.CardCount > maxDocumentCards {
		writeError(w, http.StatusBadRequest, fmt.Errorf("документ слишком велик: не более %d графических объектов и %d карточек", maxDocumentObjects, maxDocumentCards))
		return
	}
	pouIDs := make([]*int64, len(request.POUs))
	for index := range request.POUs {
		pouIDs[index] = request.POUs[index].POUID
	}
	var result generator.Result
	_, err = s.allocator.WithReservation(requirements.T11Count, requirements.CardCount, requirements.POUCount, generator.ReservationOptions{
		T11Start: request.T11Start, CardStart: request.CardStart, POUIDs: pouIDs,
	}, func(ids generator.IDRange) error {
		var generateErr error
		result, generateErr = s.generator.GenerateDocument(request, resolved, ids)
		return generateErr
	})
	if err != nil {
		var persistenceErr *generator.AllocatorPersistenceError
		if errors.As(err, &persistenceErr) {
			writeError(w, http.StatusInternalServerError, err)
		} else {
			writeError(w, http.StatusBadRequest, err)
		}
		return
	}
	fallback := "multi_pou"
	if len(resolved) == 1 && len(resolved[0].Signals) == 1 {
		fallback = resolved[0].Signals[0].Ref.Template.Name
	}
	s.writeGeneratedResult(w, request.FileName, fallback, result)
}

func hasLegacyGenerateFields(request generator.Request) bool {
	return strings.TrimSpace(request.TemplateKey) != "" ||
		strings.TrimSpace(request.ObjectName) != "" ||
		strings.TrimSpace(request.POUName) != "" ||
		strings.TrimSpace(request.NameMode) != "" ||
		strings.TrimSpace(request.Description) != "" ||
		strings.TrimSpace(request.ClusterPath) != "" ||
		request.OffsetX != nil || request.OffsetY != nil || request.POUID != nil || request.POUGroupID != nil || request.POUNumber != nil
}

func (s *Server) writeGeneratedResult(w http.ResponseWriter, requestedName, fallbackName string, result generator.Result) {
	fileName := safeOutputName(requestedName, result.BaseName, fallbackName)
	s.outputMu.Lock()
	defer s.outputMu.Unlock()
	fileName, err := uniqueOutputName(s.outputDir, fileName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := writeAtomic(filepath.Join(s.outputDir, fileName), result.XML); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("записать результат: %w", err))
		return
	}
	s.logger.Printf("generated %s with %d POU and %d signals", fileName, result.Summary.POUCount, result.Summary.SignalCount)
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
		if entry.IsDir() || !supportedOutputFile(entry.Name()) {
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
	if err != nil || filepath.Base(name) != name || !supportedOutputFile(name) {
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
	if strings.EqualFold(filepath.Ext(name), ".xls") {
		w.Header().Set("Content-Type", "application/vnd.ms-excel")
	} else {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	_, _ = io.Copy(w, file)
}

func supportedOutputFile(name string) bool {
	extension := filepath.Ext(name)
	return strings.EqualFold(extension, ".xml") || strings.EqualFold(extension, ".xls")
}

func (s *Server) localPOST(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || parsed == nil || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") {
				writeError(w, http.StatusForbidden, fmt.Errorf("запрос отклонён проверкой Origin"))
				return
			}
		}
		next(w, r)
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	defer r.Body.Close()
	const maxJSONBodyBytes int64 = 8 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	decoder := json.NewDecoder(r.Body)
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
