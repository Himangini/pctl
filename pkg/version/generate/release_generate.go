// +build release

package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/blang/semver"
	"github.com/dave/jennifer/jen"

	"github.com/weaveworks/pctl/pkg/version"
)

const versionFilename = "pkg/version/release.go"
const defaultPreReleaseID = "dev"
const defaultReleaseCandidate = "rc.0"

func main() {
	if len(os.Args) < 2 {
		log.Fatal("missing argument")
	}

	command := os.Args[1]

	var newVersion, newPreRelease string
	switch command {
	case "release":
		newVersion, newPreRelease = prepareRelease()
	case "release-candidate":
		newVersion, newPreRelease = prepareReleaseCandidate()
	case "development":
		newVersion, newPreRelease = nextDevelopmentIteration()
	case "full-version":
		fmt.Println(version.GetVersion())
		return
	case "print-version":
		// Print simplified version X.Y.Z
		fmt.Println(version.Version)
		return
	case "print-major-minor-version":
		fmt.Println(printMajorMinor())
		return
	default:
		log.Fatalf("unknown option %q. Expected 'release', 'release-candidate', 'development', 'print-version' or 'print-major-minor-version'", command)
	}

	if err := writeVersionToFile(newVersion, newPreRelease, versionFilename); err != nil {
		log.Fatalf("unable to write file: %s", err.Error())
	}

	version.Version = newVersion
	version.PreReleaseID = newPreRelease
	fmt.Println(version.GetVersion())
}

func prepareRelease() (string, string) {
	return version.Version, ""
}

func prepareReleaseCandidate() (string, string) {
	if strings.HasPrefix(version.PreReleaseID, "rc.") {
		// Next RC
		rcNumber, err := strconv.Atoi(strings.TrimPrefix(version.PreReleaseID, "rc."))
		if err != nil {
			log.Fatalf("cannot parse rc version from pre-release id %s", version.PreReleaseID)
		}
		newRC := rcNumber + 1
		return version.Version, fmt.Sprintf("rc.%d", newRC)
	}
	return version.Version, defaultReleaseCandidate
}

func printMajorMinor() string {
	ver := semver.MustParse(version.Version)
	return fmt.Sprintf("%v.%v", ver.Major, ver.Minor)
}

func nextDevelopmentIteration() (string, string) {
	ver := semver.MustParse(version.Version)
	ver.Minor++
	return ver.String(), defaultPreReleaseID
}

func writeVersionToFile(version, preReleaseID, fileName string) error {
	f := jen.NewFilePath("pkg/version")

	f.Comment("This file was generated by release_generate.go; DO NOT EDIT.")
	f.Line()

	f.Comment("Version is the version number in semver format X.Y.Z")
	f.Var().Id("Version").Op("=").Lit(version)

	f.Comment("PreReleaseID can be empty for releases, \"rc.X\" for release candidates and \"dev\" for snapshots")
	f.Var().Id("PreReleaseID").Op("=").Lit(preReleaseID)

	f.Comment("gitCommit is the short commit hash. It will be set by the linker.")
	f.Var().Id("gitCommit").Op("=").Lit("")

	f.Comment("buildDate is the time of the build with format yyyy-mm-ddThh:mm:ssZ. It will be set by the linker.")
	f.Var().Id("buildDate").Op("=").Lit("")

	if err := f.Save(fileName); err != nil {
		return err
	}
	return nil
}
