module znkr.io/writst

go 1.26.0

tool (
	golang.org/x/tools/cmd/stringer
	znkr.io/writst/internal/fieldaccessgen
)

require (
	github.com/google/go-cmp v0.7.0
	github.com/woodsbury/decimal128 v1.4.0
	znkr.io/diff v1.0.1
)

require (
	golang.org/x/mod v0.35.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/tools v0.44.0 // indirect
)
