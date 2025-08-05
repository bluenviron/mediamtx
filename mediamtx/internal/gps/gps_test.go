package gps

import (
	"testing"
	"time"
)

func TestDataCreation(t *testing.T) {
	data := &Data{
		Timestamp: time.Now(),
		Latitude:  12.345678,
		Longitude: 123.456789,
		Status:    "A",
	}
	
	if data.Latitude != 12.345678 {
		t.Errorf("Expected Latitude to be 12.345678, got %f", data.Latitude)
	}
	
	if data.Status != "A" {
		t.Errorf("Expected Status to be A, got %s", data.Status)
	}
}

func TestFormatCoordinate(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{12.345678, "12.345678"},
		{123.456789, "123.456789"},
		{-45.123456, "-45.123456"},
		{0.0, "0.000000"},
	}
	
	for _, test := range tests {
		result := FormatCoordinate(test.input)
		if result != test.expected {
			t.Errorf("FormatCoordinate(%f) = %s, expected %s", test.input, result, test.expected)
		}
	}
}

// MockManager is a mock implementation of Manager for testing.
type MockManager struct {
	data *Data
}

func NewMockManager() *MockManager {
	return &MockManager{
		data: &Data{
			Timestamp: time.Now(),
			Latitude:  37.5665,
			Longitude: 126.9780,
			Status:    "A",
		},
	}
}

func (m *MockManager) GetCurrentGPS() *Data {
	return m.data
}

func (m *MockManager) Close() error {
	return nil
}

func TestMockManager(t *testing.T) {
	manager := NewMockManager()
	defer manager.Close()
	
	data := manager.GetCurrentGPS()
	if data == nil {
		t.Fatal("Expected GPS data, got nil")
	}
	
	if data.Latitude != 37.5665 {
		t.Errorf("Expected Latitude to be 37.5665, got %f", data.Latitude)
	}
	
	if data.Status != "A" {
		t.Errorf("Expected Status to be A, got %s", data.Status)
	}
}