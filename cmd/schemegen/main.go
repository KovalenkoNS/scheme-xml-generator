package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"scheme-xml-generator/internal/appserver"
	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/generator"
	"scheme-xml-generator/internal/library"
	webui "scheme-xml-generator/web"
	"syscall"
	"time"
)

func main() {
	rootOption := flag.String("root", "", "application data directory")
	portOption := flag.Int("port", 0, "override HTTP port")
	noBrowser := flag.Bool("no-browser", false, "do not open the browser automatically")
	flag.Parse()

	root, err := applicationRoot(*rootOption)
	if err != nil {
		log.Fatal(err)
	}
	for _, directory := range []string{"libraries", "output", "data", "logs"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			log.Fatal(err)
		}
	}
	logFile, err := os.OpenFile(filepath.Join(root, "logs", "application.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	logger := log.New(logFile, "", log.Ldate|log.Ltime|log.LUTC)

	settings, err := config.Load(filepath.Join(root, "config.json"))
	if err != nil {
		log.Fatal(err)
	}
	if *portOption != 0 {
		if *portOption < 1 || *portOption > 65535 {
			log.Fatal("-port должен быть в диапазоне 1..65535")
		}
		settings.Port = *portOption
	}
	repository := library.NewRepository(filepath.Join(root, "libraries"))
	catalog, err := repository.Refresh()
	if err != nil {
		log.Fatal(err)
	}
	logger.Printf("loaded %d libraries, %d templates, %d errors", len(catalog.Libraries), len(catalog.Templates), len(catalog.Errors))
	allocator, err := generator.NewAllocator(filepath.Join(root, "data", "state.json"), settings.IDs)
	if err != nil {
		log.Fatal(err)
	}
	application := appserver.New(repository, generator.Generator{Config: settings}, allocator, filepath.Join(root, "output"), webui.Files(), logger)
	address := fmt.Sprintf("%s:%d", settings.ListenAddress, settings.Port)
	httpServer := &http.Server{Addr: address, Handler: application.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	pageURL := fmt.Sprintf("http://%s:%d", settings.ListenAddress, settings.Port)
	if settings.AutoOpen && !*noBrowser {
		go func() {
			time.Sleep(450 * time.Millisecond)
			if err := openBrowser(pageURL); err != nil {
				logger.Printf("open browser: %v", err)
			}
		}()
	}
	go func() {
		fmt.Printf("XML Scheme Generator: %s\n", pageURL)
		fmt.Printf("Libraries: %s\n", filepath.Join(root, "libraries"))
		fmt.Printf("Output: %s\n", filepath.Join(root, "output"))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = appserver.Shutdown(shutdown, httpServer)
}

func applicationRoot(option string) (string, error) {
	if option != "" {
		return filepath.Abs(option)
	}
	if value := os.Getenv("SCHEME_XML_GENERATOR_HOME"); value != "" {
		return filepath.Abs(value)
	}
	cwd, err := os.Getwd()
	if err == nil {
		if _, statErr := os.Stat(filepath.Join(cwd, "libraries")); statErr == nil {
			return cwd, nil
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(executable), nil
}

func openBrowser(address string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	case "darwin":
		command = exec.Command("open", address)
	default:
		command = exec.Command("xdg-open", address)
	}
	return command.Start()
}
