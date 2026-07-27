// Package version carries the release version. Release builds bake it in with
// `-ldflags "-X github.com/luojiahai/potato/internal/version.Version=x.y.z"`
// (scripts/build.sh); source builds report "dev".
package version

var Version = "dev"
