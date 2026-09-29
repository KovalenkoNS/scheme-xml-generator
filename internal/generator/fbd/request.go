// Single-template FBD request stage: Validates the single-template request and resolves instance/POU names.
package fbd

import (
	"fmt"

	"scheme-xml-generator/internal/generator/identifiers"

	"scheme-xml-generator/internal/library"

	"strings"
)

// prepareRequest Validates the single-template request and resolves instance/POU names.
// Context changes apply only to this generator snapshot; invalid library content stops the pipeline.
func (b *singleBuild) prepareRequest() error {
	var err error
	var contextErr error
	b.generator, contextErr = b.generator.withGenerationContext(b.request.Context)
	if contextErr != nil {
		return contextErr
	}
	if len(b.request.POUs) != 0 {
		return fmt.Errorf("Generate поддерживает только legacy-запрос одного POU; для поля pous используйте GenerateDocument")
	}
	if supported, issues := library.TemplateCompatibility(b.ref); !supported {
		return fmt.Errorf("шаблон несовместим с генератором: %s", strings.Join(issues, "; "))
	}
	b.preview, err = PreviewName(b.ref, b.request.ObjectName, b.request.NameMode)
	if err != nil {
		return err
	}
	b.pouName, err = resolvePOUName(b.request.POUName, b.ref.Template.Name)
	if err != nil {
		return err
	}
	if err := identifiers.ValidateText(b.request.Description, "описание"); err != nil {
		return err
	}
	if err := identifiers.ValidateText(b.request.ClusterPath, "KLPath"); err != nil {
		return err
	}
	return nil
}
