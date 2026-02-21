module znkr.io/writst

go 1.26.0

tool (
	golang.org/x/tools/cmd/stringer
	znkr.io/writst/ir/internal/fieldaccessgen
)

require (
	github.com/google/go-cmp v0.7.0
	znkr.io/diff v1.0.0-beta.4
)

require (
	github.com/woodsbury/decimal128 v1.4.0 // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
)
