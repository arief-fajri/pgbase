module github.com/arief-fajri/pgbase

go 1.25.0

toolchain go1.25.14

require (
	github.com/disintegration/imaging v1.6.2
	github.com/domodwyer/mailyak/v3 v3.6.2
	github.com/dop251/goja v0.0.0-20260722130236-0768e0998ac0
	github.com/dop251/goja_nodejs v0.0.0-20260212111938-1f56ff5bcf14
	github.com/fatih/color v1.19.0
	github.com/fsnotify/fsnotify v1.10.1
	github.com/gabriel-vasile/mimetype v1.4.13
	github.com/ganigeorgiev/fexpr v0.6.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/jackc/pgx/v5 v5.9.2
	github.com/microcosm-cc/bluemonday v1.0.27
	github.com/pocketbase/dbx v1.12.0
	github.com/pocketbase/ozzo-validation/v4 v4.3.0
	github.com/pocketbase/tygoja v0.1.0
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_model v0.6.2
	github.com/spf13/cast v1.10.0
	github.com/spf13/cobra v1.10.2
	golang.org/x/crypto v0.55.0
	golang.org/x/image v0.45.0
	golang.org/x/net v0.58.0
	golang.org/x/oauth2 v0.36.0
	golang.org/x/sync v0.22.0
	modernc.org/sqlite v1.57.0
)

require (
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dlclark/regexp2/v2 v2.5.2 // indirect
	github.com/dop251/base64dec v0.0.0-20231022112746-c6c9f9a96217 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20260802141513-ef3492d7dac3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace github.com/pocketbase/dbx => ./third_party/dbx
