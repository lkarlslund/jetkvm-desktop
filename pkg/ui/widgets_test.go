package ui

import "testing"

func TestSliderMeasure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		slider      Slider
		constraints Constraints
		want        Size
	}{
		{
			name:        "default width and height",
			slider:      Slider{},
			constraints: NewConstraints(400, 100),
			want:        Size{W: 140, H: 28},
		},
		{
			name:        "custom min width",
			slider:      Slider{MinW: 220},
			constraints: NewConstraints(400, 100),
			want:        Size{W: 220, H: 28},
		},
		{
			name:        "clamped to constraints",
			slider:      Slider{MinW: 220},
			constraints: NewConstraints(180, 24),
			want:        Size{W: 180, H: 24},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.slider.Measure(nil, tt.constraints); got != tt.want {
				t.Fatalf("Measure() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSliderClampValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    float64
		minValue float64
		maxValue float64
		want     float64
	}{
		{name: "below range", value: -5, minValue: 0, maxValue: 10, want: 0},
		{name: "in range", value: 4, minValue: 0, maxValue: 10, want: 4},
		{name: "above range", value: 15, minValue: 0, maxValue: 10, want: 10},
		{name: "invalid range", value: 15, minValue: 7, maxValue: 7, want: 7},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := SliderClampValue(tt.value, tt.minValue, tt.maxValue); got != tt.want {
				t.Fatalf("SliderClampValue(%v, %v, %v) = %v, want %v", tt.value, tt.minValue, tt.maxValue, got, tt.want)
			}
		})
	}
}

func TestSliderSnapValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    float64
		minValue float64
		maxValue float64
		step     float64
		want     float64
	}{
		{name: "no step", value: 3.2, minValue: 0, maxValue: 10, step: 0, want: 3.2},
		{name: "snap down", value: 3.2, minValue: 0, maxValue: 10, step: 1, want: 3},
		{name: "snap up", value: 3.6, minValue: 0, maxValue: 10, step: 1, want: 4},
		{name: "offset range", value: 18, minValue: 10, maxValue: 30, step: 5, want: 20},
		{name: "clamped to max", value: 42, minValue: 10, maxValue: 30, step: 5, want: 30},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := SliderSnapValue(tt.value, tt.minValue, tt.maxValue, tt.step); got != tt.want {
				t.Fatalf("SliderSnapValue(%v, %v, %v, %v) = %v, want %v", tt.value, tt.minValue, tt.maxValue, tt.step, got, tt.want)
			}
		})
	}
}

func TestSliderValueAt(t *testing.T) {
	t.Parallel()

	bounds := Rect{X: 20, Y: 0, W: 200, H: 28}

	tests := []struct {
		name     string
		x        float64
		minValue float64
		maxValue float64
		step     float64
		want     float64
	}{
		{name: "before track", x: 0, minValue: 10, maxValue: 30, step: 0, want: 10},
		{name: "track start", x: 30, minValue: 10, maxValue: 30, step: 0, want: 10},
		{name: "midpoint", x: 120, minValue: 10, maxValue: 30, step: 0, want: 20},
		{name: "after track", x: 250, minValue: 10, maxValue: 30, step: 0, want: 30},
		{name: "snapped midpoint", x: 120, minValue: 0, maxValue: 100, step: 25, want: 50},
		{name: "snapped uneven position", x: 102, minValue: 0, maxValue: 100, step: 25, want: 50},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := SliderValueAt(bounds, tt.x, tt.minValue, tt.maxValue, tt.step); got != tt.want {
				t.Fatalf("SliderValueAt(%+v, %v, %v, %v, %v) = %v, want %v", bounds, tt.x, tt.minValue, tt.maxValue, tt.step, got, tt.want)
			}
		})
	}
}

func TestSliderValueAtZeroWidthTrack(t *testing.T) {
	t.Parallel()

	bounds := Rect{X: 10, Y: 0, W: 20, H: 28}
	if got := SliderValueAt(bounds, 20, 10, 30, 5); got != 10 {
		t.Fatalf("SliderValueAt(%+v, 20, 10, 30, 5) = %v, want 10", bounds, got)
	}
}

func TestSliderRangeValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		slider Slider
		min    float64
		max    float64
	}{
		{name: "custom range", slider: Slider{Min: 10, Max: 30}, min: 10, max: 30},
		{name: "invalid range falls back", slider: Slider{Min: 5, Max: 5}, min: 0, max: 1},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			minValue, maxValue := tt.slider.rangeValues()
			if minValue != tt.min || maxValue != tt.max {
				t.Fatalf("rangeValues() = (%v, %v), want (%v, %v)", minValue, maxValue, tt.min, tt.max)
			}
		})
	}
}
