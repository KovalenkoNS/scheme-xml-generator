// Чтение контракта ST-перекладки и запрет встроенного FBD до разбора инженерного источника.
package moduleassignment

import (
	"fmt"
	"net/http"
	"scheme-xml-generator/internal/generator"
	modulemapping "scheme-xml-generator/internal/httpapi/io/modulemapping"
)

type assignmentInput struct {
	Data                 []byte
	FileName, RawContext string
	Request              generator.ModuleMappingRequest
}

// readAssignmentInput читает ограниченный XLSX multipart и настройки выбранных модулей.
// Возвращает HTTP-вход либо статус ошибки; FBD получает 410 до парсинга карты, allocator и output.
func readAssignmentInput(w http.ResponseWriter, r *http.Request) (assignmentInput, int, error) {
	data, fileName, rawContext, err := modulemapping.ReadModuleMappingUpload(w, r)
	if err != nil {
		return assignmentInput{}, http.StatusBadRequest, err
	}
	request, err := decodeModuleAssignmentRequest(r.MultipartForm.Value["config"])
	if err != nil {
		return assignmentInput{}, http.StatusBadRequest, err
	}
	if request.Kind == "fbd" || request.Kind == "fbd-native-do" {
		return assignmentInput{}, http.StatusGone, fmt.Errorf("FBD создаётся только из подключённой библиотеки. Выберите шаблон на основной странице.")
	}

	return assignmentInput{Data: data, FileName: fileName, RawContext: rawContext, Request: request}, 0, nil
}
