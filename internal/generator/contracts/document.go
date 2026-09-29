// Общие пределы транспортных ID и размеров страницы применяются планировщиками и XML-генераторами.
package contracts

const (
	DefaultSignalOffsetX = 300
	DefaultSignalOffsetY = 100
	DefaultSignalGapY    = 20
	MaxPageExtent        = 1000000
	MaxTransportID       = int64(2147483647)
)
