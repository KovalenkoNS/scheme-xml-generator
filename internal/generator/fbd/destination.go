// Single-template FBD destination stage: Normalizes destination controller, resource, group and program numbers.
package fbd

import (
	"fmt"
	"scheme-xml-generator/internal/generator/addressing"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	"strconv"
)

// prepareDestination Normalizes destination controller, resource, group and program numbers.
// Request overrides preserve the previous import rules and do not update application settings.
func (b *singleBuild) prepareDestination() error {
	var err error
	b.controllerID, err = addressing.NormalizeContextInteger(b.generator.Config.Common.ControllerID, "ControllerID")
	if err != nil {
		return err
	}
	b.resourceID, err = addressing.NormalizeContextInteger(b.generator.Config.Common.ResourceID, "ResuorceID")
	if err != nil {
		return err
	}
	b.groupID, err = addressing.NormalizeContextInteger(b.generator.Config.Page.GroupID, "POU GroupID")
	if err != nil {
		return err
	}
	b.pouNumber, err = addressing.NormalizeContextInteger(b.generator.Config.Page.POUNumber, "POUNum")
	if err != nil {
		return err
	}
	if b.request.POUGroupID != nil {
		if *b.request.POUGroupID < 1 || *b.request.POUGroupID > xmlidentity.MaxTransportID {
			return fmt.Errorf("POU GroupID должен быть положительным signed 32-bit")
		}
		b.groupID = strconv.FormatInt(*b.request.POUGroupID, 10)
	}
	if b.request.POUNumber != nil {
		if *b.request.POUNumber < 1 || *b.request.POUNumber > xmlidentity.MaxTransportID {
			return fmt.Errorf("POUNum должен быть положительным signed 32-bit")
		}
		b.pouNumber = strconv.FormatInt(*b.request.POUNumber, 10)
	}

	return nil
}
