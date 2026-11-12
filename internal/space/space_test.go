package space

import (
	"testing"
	"time"
)

func TestParseAir(t *testing.T) {
	a, err := ParseAir([]byte(`{"product":"astro","init":"2026111212","dataseries":[{"timepoint":3,"cloudcover":2,"seeing":1,"transparency":8},{"timepoint":6,"cloudcover":9,"seeing":5,"transparency":3}]}`))
	if err != nil || len(a.Hours) != 2 {
		t.Fatal(err, a)
	}
	if h := a.Hours[0]; h.Seeing != 5 || h.Transparency != 1 || !h.Time.Equal(time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC)) {
		t.Fatalf("%+v", h)
	}
	if a.At(time.Date(2026, 11, 12, 18, 30, 0, 0, time.UTC)).Seeing != 3 {
		t.Fatal("at")
	}
}

func TestKpAndPlaces(t *testing.T) {
	kp, err := ParseKp([]byte(`[{"time_tag":"2026-11-12T18:00:00","kp":5.33,"observed":"predicted"},{"time_tag":"2026-11-12T21:00:00","kp":7,"observed":"predicted"}]`))
	if err != nil || len(kp) != 2 {
		t.Fatal(err)
	}
	rows, err := ParseKp([]byte(`[["time_tag","kp","observed","noaa_scale"],["2026-11-12 18:00:00","4.67","observed",null]]`))
	if err != nil || len(rows) != 1 || rows[0].Kp != 4.67 || !rows[0].Observed {
		t.Fatalf("rows: %v %v", rows, err)
	}
	a := &Aurora{Kp: kp}
	if m := a.MaxKp(time.Date(2026, 11, 12, 20, 0, 0, 0, time.UTC), time.Date(2026, 11, 13, 4, 0, 0, 0, time.UTC)); m != 7 {
		t.Fatalf("max %v", m)
	}
	if k := KpNeeded(69.65, 18.96); k > 1 { // Tromsø: most nights
		t.Errorf("Tromsø needs %v", k)
	}
	if k := KpNeeded(43.86, 18.41); k < 7 { // Sarajevo: only the great storms (May 2024)
		t.Errorf("Sarajevo needs %v", k)
	}
	g, _ := ParseOvation([]byte(`{"coordinates":[[19,70,35],[18,44,0]]}`))
	a.grid = g
	if a.Chance(69.6, 18.9) != 35 || a.Chance(44, 18) != 0 {
		t.Fatal("chance")
	}
}
