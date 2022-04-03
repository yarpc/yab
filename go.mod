module github.com/yarpc/yab

go 1.14

require (
	github.com/BurntSushi/toml v1.0.0 // indirect
	github.com/HdrHistogram/hdrhistogram-go v1.1.2 // indirect
	github.com/anmitsu/go-shlex v0.0.0-20161002113705-648efa622239 // indirect
	github.com/apache/thrift v0.0.0-20161221203622-b2a4d4ae21c7
	github.com/cactus/go-statsd-client/statsd v0.0.0-20190922033735-5ca90424ceb7
	github.com/casimir/xdg-go v0.0.0-20160329195404-372ccc2180da
	github.com/fatih/structtag v1.0.0 // indirect
	github.com/flynn/go-shlex v0.0.0-20150515145356-3f9db97f8568 // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/gogo/googleapis v1.3.1-0.20190914144012-b8d18e97a9a1 // indirect
	github.com/gogo/status v1.1.0 // indirect
	github.com/golang/mock v1.3.1-0.20190508161146-9fa652df1129 // indirect
	github.com/golang/protobuf v1.3.2
	github.com/jessevdk/go-flags v1.4.1-0.20180331124232-1c38ed7ad0cc
	github.com/jhump/protoreflect v0.0.0-20180908113807-a84568470d8a
	github.com/koalacxr/quantile v0.0.1 // indirect
	github.com/kr/pretty v0.2.0 // indirect
	github.com/opentracing/opentracing-go v1.2.0
	github.com/pkg/errors v0.8.1 // indirect
	github.com/prometheus/client_golang v1.1.0 // indirect
	github.com/prometheus/procfs v0.0.5 // indirect
	github.com/stretchr/testify v1.7.0
	github.com/uber-go/mapdecode v1.0.0 // indirect
	github.com/uber/jaeger-client-go v2.30.0+incompatible
	github.com/uber/tchannel-go v1.22.3
	go.uber.org/atomic v1.9.0
	go.uber.org/fx v1.10.0 // indirect
	go.uber.org/multierr v1.8.0
	go.uber.org/net/metrics v1.2.0 // indirect
	go.uber.org/thriftrw v1.20.1
	go.uber.org/yarpc v1.40.0
	go.uber.org/zap v1.10.0
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616 // indirect
	golang.org/x/net v0.0.0-20220401154927-543a649e0bdd
	golang.org/x/sys v0.0.0-20220403020550-483a9cbc67c0 // indirect
	google.golang.org/genproto v0.0.0-20180817151627-c66870c02cf8
	google.golang.org/grpc v1.24.0
	gopkg.in/yaml.v2 v2.4.0
	honnef.co/go/tools v0.3.0 // indirect
)

replace github.com/streadway/quantile => github.com/koalacxr/quantile v0.0.1
