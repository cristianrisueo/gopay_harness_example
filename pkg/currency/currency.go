package currency

// Currency represents supported fiat currencies
type Currency string

const (
	EUR Currency = "EUR"
	USD Currency = "USD"
	JPY Currency = "JPY"
)

// IsValid checks if the currency is supported
func (c Currency) IsValid() bool {
	return c == EUR || c == USD || c == JPY
}

// String returns the string representation
func (c Currency) String() string {
	return string(c)
}
