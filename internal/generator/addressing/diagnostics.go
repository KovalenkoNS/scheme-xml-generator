// Import addressing validates transport values and confirmed physical driver profiles.
package addressing

import (
	"fmt"
)

// DODiagnosticTag Строит аппаратную ссылку диагностики DO32P по фактическому ModuleID.
// Используется цепью Quality модульного FBD-профиля CPU850.
func DODiagnosticTag(id int64) string {
	return fmt.Sprintf("_IO_I%d_DO32P_0_VAL_DIAG", id)
}
