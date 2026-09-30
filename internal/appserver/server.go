// Package appserver связывает предметные HTTP-компоненты независимого генератора.
package appserver

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/generator/allocation"
	fbdgen "scheme-xml-generator/internal/generator/fbd"
	hmigen "scheme-xml-generator/internal/generator/hmi"
	stgen "scheme-xml-generator/internal/generator/st"
	"scheme-xml-generator/internal/httpapi/catalog"
	"scheme-xml-generator/internal/httpapi/fbd"
	"scheme-xml-generator/internal/httpapi/hmi"
	hostapi "scheme-xml-generator/internal/httpapi/host"
	ioao "scheme-xml-generator/internal/httpapi/io/ao"
	"scheme-xml-generator/internal/httpapi/io/modulemapping"
	iosource "scheme-xml-generator/internal/httpapi/io/source"
	"scheme-xml-generator/internal/httpapi/output"
	"scheme-xml-generator/internal/httpapi/st"
	"scheme-xml-generator/internal/httpapi/st/moduleassignment"
	techobjectapi "scheme-xml-generator/internal/httpapi/techobjects"
	"scheme-xml-generator/internal/httpapi/transport"
	"scheme-xml-generator/internal/httpapi/workspace"
	"scheme-xml-generator/internal/integration/hostclient"
	"scheme-xml-generator/internal/library"
)

type Server struct {
	repository *library.Repository
	config     config.Config
	allocator  *allocation.Allocator
	output     *output.Store
	staticFS   fs.FS
	host       *hostclient.Client
}

// New получает зависимости независимого генератора при запуске приложения.
// Возвращает сборщик HTTP-компонентов; каталог вывода обслуживается единственным output.Store.
func New(repository *library.Repository, settings config.Config, allocator *allocation.Allocator, outputDir string, staticFS fs.FS, logger *log.Logger) *Server {
	return &Server{repository: repository, config: settings, allocator: allocator, output: output.New(outputDir, logger), staticFS: staticFS, host: hostclient.FromEnvironment()}
}

// Handler собирает предметные HTTP-сервисы с текущими настройками и прежними адресами API.
// Возвращает маршрутизатор с защитными заголовками; обработка библиотеки, IO и XML живёт в отдельных пакетах.
func (s *Server) Handler() http.Handler {
	libraryAPI := catalog.Service{Repository: s.repository}
	workspaceAPI := workspace.Service{Config: &s.config, Host: s.host}
	hostAPI := hostapi.Service{Client: s.host}
	fbdAPI := fbd.Service{Repository: s.repository, Generator: &fbdgen.Generator{Config: s.config}, Allocator: s.allocator, Output: s.output}
	stAPI := st.Service{Generator: &stgen.Generator{Config: s.config}, Allocator: s.allocator, Output: s.output}
	hmiAPI := hmi.Service{Generator: &hmigen.Generator{Config: s.config}, Allocator: s.allocator, Output: s.output}
	aoAPI := ioao.Service{}
	sourceAPI := iosource.Service{}
	moduleMapAPI := modulemapping.Service{}
	mappingAPI := moduleassignment.Service{Allocator: s.allocator, Output: s.output, ST: &stAPI}
	objectsAPI := techobjectapi.Service{Output: s.output}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		transport.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /api/templates", libraryAPI.HandleTemplates)
	mux.HandleFunc("GET /api/workspace", workspaceAPI.HandleWorkspace)
	mux.HandleFunc("GET /api/host/session", hostAPI.HandleSession)
	mux.HandleFunc("POST /api/libraries/import", transport.LocalPOST(libraryAPI.HandleLibraryImport))
	mux.HandleFunc("POST /api/refresh", transport.LocalPOST(libraryAPI.HandleRefresh))
	mux.HandleFunc("POST /api/preview-name", transport.LocalPOST(fbdAPI.HandlePreviewName))
	mux.HandleFunc("POST /api/generate", transport.LocalPOST(fbdAPI.HandleGenerate))
	mux.HandleFunc("POST /api/generate/library-do", transport.LocalPOST(fbdAPI.HandleLibraryDO))
	mux.HandleFunc("GET /api/temporary/ao/profile", aoAPI.HandleAOProfile)
	mux.HandleFunc("POST /api/temporary/ao/preview", transport.LocalPOST(aoAPI.HandleAOPreview))
	mux.HandleFunc("POST /api/temporary/ao/generate", transport.LocalPOST(fbdAPI.HandleAOGenerate))
	mux.HandleFunc("POST /api/temporary/ao/generate-st", transport.LocalPOST(stAPI.HandleAOSTGenerate))
	mux.HandleFunc("POST /api/temporary/ao/generate-diagnostic", transport.LocalPOST(hmiAPI.HandleAODiagnosticGenerate))
	mux.HandleFunc("POST /api/temporary/diagnostic/preview", transport.LocalPOST(sourceAPI.HandlePLCDiagnosticPreview))
	mux.HandleFunc("POST /api/temporary/diagnostic/generate", transport.LocalPOST(hmiAPI.HandlePLCDiagnosticGenerate))
	mux.HandleFunc("POST /api/techobjects/preview", transport.LocalPOST(objectsAPI.HandleTechObjectsPreview))
	mux.HandleFunc("POST /api/techobjects/generate", transport.LocalPOST(objectsAPI.HandleTechObjectsGenerate))
	mux.HandleFunc("GET /api/mappings/profile", moduleMapAPI.HandleModuleMappingProfile)
	mux.HandleFunc("POST /api/mappings/preview", transport.LocalPOST(moduleMapAPI.HandleModuleMappingPreview))
	mux.HandleFunc("POST /api/mappings/generate", transport.LocalPOST(mappingAPI.HandleModuleAssignmentGenerate))
	// Historical URLs are aliases of the same read-only preview / ST handlers.
	mux.HandleFunc("GET /api/skz/profile", moduleMapAPI.HandleModuleMappingProfile)
	mux.HandleFunc("POST /api/skz/preview", transport.LocalPOST(moduleMapAPI.HandleModuleMappingPreview))
	mux.HandleFunc("POST /api/skz/generate", transport.LocalPOST(mappingAPI.HandleModuleAssignmentGenerate))
	mux.HandleFunc("GET /api/outputs", s.output.HandleOutputs)
	mux.HandleFunc("GET /api/output/{name}", s.output.HandleDownload)
	mux.Handle("GET /", http.FileServer(http.FS(s.staticFS)))

	return transport.SecurityHeaders(mux)
}

// Shutdown завершает HTTP-сервер по сигналу остановки приложения.
// Передаёт context в стандартный graceful shutdown и возвращает ошибку завершения.
func Shutdown(ctx context.Context, server *http.Server) error { return server.Shutdown(ctx) }
