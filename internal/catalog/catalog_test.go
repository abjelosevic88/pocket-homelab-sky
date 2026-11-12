package catalog

import "testing"

func TestConstellationAt(t *testing.T) {
	cases := []struct {
		ra, dec float64
		want    string
	}{
		{83.8, -5.4, "Ori"}, {10.68, 41.27, "And"}, {279.2, 38.8, "Lyr"}, {201.3, -11.2, "Vir"}, {88.8, 7.4, "Ori"},
		{37.95, 89.26, "UMi"}, {0, 89.9, "UMi"}, {120, -89.9, "Oct"}, {101.29, -16.72, "CMa"}, {0.5, 10, "Psc"}, {359.5, 30, "Peg"}, {5, 35, "And"},
	}
	for _, c := range cases {
		if got := ConstellationAt(c.ra, c.dec); got != c.want {
			t.Errorf("%.2f %.2f: %q want %q", c.ra, c.dec, got, c.want)
		}
	}
	for _, m := range Messier() {
		if m.Con == "" {
			t.Errorf("%s has no constellation", m.ID)
		}
	}
	if m := Messier()[30]; m.ID != "M31" || m.Con != "And" {
		t.Errorf("M31: %+v", m)
	}
}

func TestCatalogLoads(t *testing.T) {
	if n := len(Stars()); n < 5000 {
		t.Fatalf("%d stars", n)
	}
	if n := len(Constellations()); n < 88 {
		t.Fatalf("%d constellations", n)
	}
	if n := len(Messier()); n != 110 {
		t.Fatalf("%d Messier objects", n)
	}
	for _, c := range Constellations() {
		if c.About == "" {
			t.Errorf("%s has no words", c.ID)
		}
	}
	b, etag, err := ClientJSON()
	if err != nil || len(b) < 100000 || etag == "" {
		t.Fatalf("client json: %d %q %v", len(b), etag, err)
	}
	s, ok := StarByHIP(32349)
	if !ok || s.Name != "Sirius" || s.Label() != "Sirius" {
		t.Fatalf("Sirius: %+v", s)
	}
}

func TestSearch(t *testing.T) {
	first := func(q string) string {
		r := Search(q, 5)
		if len(r) == 0 {
			return ""
		}
		return r[0].Key
	}
	for q, want := range map[string]string{"m31": "dso:M31", "M 42": "dso:M42", "andromeda gal": "dso:M31", "sirius": "star:32349", "orion": "con:Ori", "bootes": "con:Boo", "pleiades": "dso:M45", "double cluster": "dso:double-cluster", "vega": "star:91262"} {
		if got := first(q); got != want {
			t.Errorf("%q: %q want %q (%v)", q, got, want, Search(q, 3))
		}
	}
	if o, ok := Lookup("dso:m13"); !ok || o.Con != "Her" {
		t.Errorf("m13: %+v", o)
	}
	if _, ok := Lookup("star:nope"); ok {
		t.Error("bad key found")
	}
}
