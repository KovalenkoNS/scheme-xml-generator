// Открытие web-интерфейса генератора системным обработчиком URL.
package main

import (
	"os/exec"

	"runtime"
)

// openBrowser открывает локальную страницу генератора после запуска HTTP-сервера.
// Передаёт URL системному обработчику Windows/macOS/Linux и возвращает ошибку запуска процесса.
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
