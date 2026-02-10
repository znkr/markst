package ir

type Unit int

const (
	UnitPt Unit = iota
	UnitMm
	UnitCm
	UnitIn
	UnitDeg
	UnitRad
	UnitEm
	UnitFr
	UnitPercent
)

func ParseUnit(s string) (Unit, bool) {
	u, ok := units[s]
	return u, ok
}

func (u Unit) String() string {
	return [...]string{"pt", "mm", "cm", "in", "deg", "rad", "em", "fr", "%"}[u]
}

var units = map[string]Unit{
	"pt":  UnitPt,
	"mm":  UnitMm,
	"cm":  UnitCm,
	"in":  UnitIn,
	"deg": UnitDeg,
	"rad": UnitRad,
	"em":  UnitEm,
	"fr":  UnitFr,
	"%":   UnitPercent,
}
