module github.com/project-radius/radius

go 1.18

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.2.0
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.1.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2 v2.0.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi v0.7.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources v1.0.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions v1.0.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/servicebus/armservicebus/v2 v2.0.0-beta.1
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage v1.2.0
	github.com/Azure/go-autorest/autorest v0.11.28
	github.com/Azure/secrets-store-csi-driver-provider-azure v1.3.0
	github.com/aws/aws-sdk-go-v2 v1.17.0
	github.com/aws/aws-sdk-go-v2/config v1.15.15
	github.com/aws/aws-sdk-go-v2/credentials v1.12.10
	github.com/aws/aws-sdk-go-v2/service/cloudcontrol v1.10.5
	github.com/aws/aws-sdk-go-v2/service/cloudformation v1.22.11
	github.com/aws/aws-sdk-go-v2/service/sts v1.16.10
	github.com/aws/smithy-go v1.13.3
	github.com/charmbracelet/bubbletea v0.23.0
	github.com/charmbracelet/lipgloss v0.6.0
	github.com/dimchansky/utfbom v1.1.1
	github.com/fatih/color v1.13.0
	github.com/go-logr/logr v1.2.3
	github.com/go-logr/zapr v1.2.3
	github.com/go-openapi/errors v0.20.2
	github.com/go-openapi/jsonpointer v0.19.5
	github.com/go-openapi/loads v0.21.1
	github.com/go-openapi/runtime v0.24.1
	github.com/go-openapi/spec v0.20.6
	github.com/go-openapi/strfmt v0.21.2
	github.com/go-playground/locales v0.13.0
	github.com/go-playground/universal-translator v0.17.0
	github.com/go-playground/validator/v10 v10.4.1
	github.com/gofrs/flock v0.8.1
	github.com/golang/mock v1.6.0
	github.com/google/go-cmp v0.5.9
	github.com/google/uuid v1.3.0
	github.com/gorilla/mux v1.8.0
	github.com/gosuri/uilive v0.0.4
	github.com/marstr/randname v0.0.0-20181206212954-d5b0f288ab8c
	github.com/mattn/go-isatty v0.0.16
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mitchellh/mapstructure v1.5.0
	github.com/novln/docker-parser v1.0.0
	github.com/spaolacci/murmur3 v1.1.0
	github.com/spf13/cobra v1.5.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.10.1
	github.com/stern/stern v1.22.0
	github.com/stretchr/testify v1.8.1
	github.com/vippsas/go-cosmosdb v0.0.0-20200428065936-29dab535353d
	github.com/wI2L/jsondiff v0.2.0
	go.mongodb.org/mongo-driver v1.9.1
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.37.0
	go.opentelemetry.io/otel v1.12.0
	go.opentelemetry.io/otel/exporters/prometheus v0.34.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.12.0
	go.opentelemetry.io/otel/exporters/zipkin v1.12.0
	go.opentelemetry.io/otel/metric v0.34.0
	go.opentelemetry.io/otel/sdk v1.12.0
	go.opentelemetry.io/otel/sdk/metric v0.34.0
	go.uber.org/atomic v1.7.0
	go.uber.org/zap v1.20.0
	golang.org/x/net v0.4.0
	golang.org/x/sync v0.1.0
	golang.org/x/text v0.5.0
	gopkg.in/ini.v1 v1.66.2
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	gotest.tools v2.2.0+incompatible
	helm.sh/helm/v3 v3.10.0
	k8s.io/api v0.25.2
	k8s.io/apiextensions-apiserver v0.25.0
	k8s.io/apimachinery v0.25.2
	k8s.io/cli-runtime v0.25.2
	k8s.io/client-go v0.25.2
	k8s.io/kubectl v0.25.0
	oras.land/oras-go/v2 v2.0.0-rc.3
	sigs.k8s.io/controller-runtime v0.12.1
	sigs.k8s.io/secrets-store-csi-driver v1.2.2
)

require (
	cloud.google.com/go/compute v1.10.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.1.2 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.21 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v0.5.2 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.1.1 // indirect
	github.com/Masterminds/sprig/v3 v3.2.2 // indirect
	github.com/Masterminds/squirrel v1.5.3 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.12.9 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.24 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.18 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.11.13 // indirect
	github.com/aymanbagabas/go-osc52 v1.2.1 // indirect
	github.com/benbjohnson/clock v1.1.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.1.2 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/chai2010/gettext-go v1.0.2 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/containerd/containerd v1.6.6 // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/creack/pty v1.1.17 // indirect
	github.com/cyphar/filepath-securejoin v0.2.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/docker v20.10.17+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.6.4 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/emicklei/go-restful/v3 v3.9.0 // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/go-gorp/gorp/v3 v3.0.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/analysis v0.21.3 // indirect
	github.com/go-openapi/validate v0.22.0 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt v3.2.1+incompatible // indirect
	github.com/golang-jwt/jwt/v4 v4.4.3 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/gosuri/uitable v0.0.4 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/imdario/mergo v0.3.13 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jmoiron/sqlx v1.3.5 // indirect
	github.com/jonboulle/clockwork v0.2.2 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lann/builder v0.0.0-20180802200727-47ae307949d0 // indirect
	github.com/lann/ps v0.0.0-20150810152359-62de8c46ede0 // indirect
	github.com/leodido/go-urn v1.2.0 // indirect
	github.com/lib/pq v1.10.6 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/magiconair/properties v1.8.5 // indirect
	github.com/marstr/collection v1.0.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/muesli/ansi v0.0.0-20221106050444-61f0cd9a192a // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/reflow v0.3.0 // indirect
	github.com/muesli/termenv v0.13.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/opencontainers/distribution-spec/specs-go v0.0.0-20220620172159-4ab4752c3b86 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.3-0.20211202183452-c5a74bcca799 // indirect
	github.com/openzipkin/zipkin-go v0.4.1 // indirect
	github.com/oras-project/artifacts-spec v1.0.0-rc.2 // indirect
	github.com/pelletier/go-toml v1.9.4 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pkg/browser v0.0.0-20210115035449-ce105d075bb4 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.39.0 // indirect
	github.com/prometheus/procfs v0.9.0 // indirect
	github.com/rivo/uniseg v0.4.2 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sahilm/fuzzy v0.1.0 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/soheilhy/cmux v0.1.5 // indirect
	github.com/spf13/afero v1.6.0 // indirect
	github.com/spf13/cast v1.4.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/subosito/gotenv v1.2.0 // indirect
	github.com/tidwall/gjson v1.14.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/tmc/grpc-websocket-proxy v0.0.0-20201229170055-e5319fda7802 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/xiang90/probing v0.0.0-20190116061207-43a291ad63a2 // indirect
	go.etcd.io/bbolt v1.3.6 // indirect
	go.etcd.io/etcd/api/v3 v3.5.6 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.6 // indirect
	go.etcd.io/etcd/client/v2 v2.305.6 // indirect
	go.etcd.io/etcd/pkg/v3 v3.5.6 // indirect
	go.etcd.io/etcd/raft/v3 v3.5.6 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.28.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.3.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.3.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.3.0 // indirect
	go.opentelemetry.io/otel/trace v1.12.0 // indirect
	go.opentelemetry.io/proto/otlp v0.11.0 // indirect
	go.uber.org/goleak v1.2.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	golang.org/x/oauth2 v0.3.0 // indirect
	golang.org/x/term v0.3.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	k8s.io/apiserver v0.25.0 // indirect
	k8s.io/component-base v0.25.0 // indirect
	oras.land/oras-go v1.2.0 // indirect
	sigs.k8s.io/kustomize/kyaml v0.13.9 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

require (
	github.com/BurntSushi/toml v1.1.0 // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/agnivade/levenshtein v1.1.1
	github.com/asaskevich/govalidator v0.0.0-20210307081110-f21760c49a8d // indirect
	github.com/charmbracelet/bubbles v0.14.0
	github.com/docker/cli v20.10.17+incompatible // indirect
	github.com/docker/distribution v2.8.1+incompatible // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/exponent-io/jsonpath v0.0.0-20210407135951-1de76d718b3f // indirect
	github.com/go-errors/errors v1.4.2 // indirect
	github.com/go-openapi/jsonreference v0.20.0 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/klauspost/compress v1.15.11 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-runewidth v0.0.14 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6 // indirect
	github.com/projectcontour/contour v1.20.1
	github.com/prometheus/client_golang v1.14.0
	github.com/rubenv/sql-migrate v1.1.2 // indirect
	github.com/russross/blackfriday v1.6.0 // indirect
	github.com/shopspring/decimal v1.3.1 // indirect
	github.com/xlab/treeprint v1.1.0 // indirect
	go.etcd.io/etcd/client/v3 v3.5.6
	go.etcd.io/etcd/server/v3 v3.5.6
	go.starlark.net v0.0.0-20220817180228-f738f5508c12 // indirect
	golang.org/x/crypto v0.0.0-20221010152910-d6f0a8c073c2 // indirect
	golang.org/x/exp v0.0.0-20220722155223-a9213eeb770e
	golang.org/x/sys v0.4.0 // indirect
	golang.org/x/time v0.0.0-20220922220347-f3bd1da661af // indirect
	google.golang.org/genproto v0.0.0-20221010155953-15ba04fc1c0e // indirect
	google.golang.org/grpc v1.50.0 // indirect
	k8s.io/klog/v2 v2.80.1 // indirect
	k8s.io/kube-openapi v0.0.0-20220803164354-a70c9af30aea // indirect
	k8s.io/utils v0.0.0-20220922133306-665eaaec4324 // indirect
	sigs.k8s.io/json v0.0.0-20220713155537-f223a00ba0e2 // indirect
	sigs.k8s.io/kustomize/api v0.12.1 // indirect
)

replace cloud.google.com/go => cloud.google.com/go v0.100.2
