// Пределы BIFF8 и памяти для однолистовой XLS-книги.
package xls

const (
	MaxRows    = 65536
	MaxColumns = 256
	maxRecord  = 8224
	// Bound intermediate allocations as well as the final download size.
	maxWorkbookBytes = 256 << 20
)
