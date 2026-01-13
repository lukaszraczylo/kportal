package ui

// Terminal dimension constants
const (
	// DefaultTermWidth is the fallback terminal width when not detected
	DefaultTermWidth = 120

	// DefaultTermHeight is the fallback terminal height when not detected
	DefaultTermHeight = 40
)

// Table column constants
const (
	// Column indices in the forwards table
	ColumnContext   = 0
	ColumnNamespace = 1
	ColumnAlias     = 2
	ColumnType      = 3
	ColumnResource  = 4
	ColumnRemote    = 5
	ColumnLocal     = 6
	ColumnStatus    = 7

	// Column widths for truncation
	ColumnWidthContext   = 14
	ColumnWidthNamespace = 16
	ColumnWidthAlias     = 18
	ColumnWidthType      = 8
	ColumnWidthResource  = 20

	// Error display widths
	ErrorDisplayWidth = 118 // Slightly less than table width (120) for padding
)

// Viewport constants
const (
	// ViewportHeight is the number of items visible in list views
	ViewportHeight = 20
)

// Path display constants
const (
	// MaxPathWidth is the maximum width for displaying file paths
	MaxPathWidth = 48
)
