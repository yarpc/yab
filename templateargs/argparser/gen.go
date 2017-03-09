package argparser

//go:generate ragel -Z -G2 -o arg.go arg.rl
//go:generate gofmt -s -w arg.go
//go:generate ./generated.sh
