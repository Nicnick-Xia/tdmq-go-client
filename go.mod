module github.com/TencentCloud/tdmq-go-client

go 1.12

require (
	github.com/DataDog/zstd v1.4.6-0.20200617134701-89f69fb7df32
	github.com/apache/pulsar-client-go/oauth2 v0.0.0-20200803021613-570d5ceffa19
	github.com/beefsack/go-rate v0.0.0-20180408011153-efa7637bb9b6
	github.com/bmizerany/perks v0.0.0-20141205001514-d9a9656a3a4b
	github.com/gogo/protobuf v1.3.1
	github.com/google/uuid v1.1.1
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/klauspost/compress v1.10.8
	github.com/kr/pretty v0.2.1 // indirect
	github.com/pierrec/lz4 v2.0.5+incompatible
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.7.1
	github.com/sirupsen/logrus v1.4.2
	github.com/spaolacci/murmur3 v1.1.0
	github.com/spf13/cobra v0.0.3
	github.com/spf13/pflag v1.0.3 // indirect
	github.com/stretchr/testify v1.4.0
	github.com/yahoo/athenz v1.8.55
)

replace github.com/apache/pulsar-client-go/oauth2 => ./oauth2
