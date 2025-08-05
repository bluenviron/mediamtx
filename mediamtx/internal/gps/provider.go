package gps

// DataProvider is an interface for providing GPS data.
type DataProvider interface {
	GetCurrentGPS() *Data
}